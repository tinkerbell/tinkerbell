package stateless

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	tbotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	v6 "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6/handler/stateless"

const clientEnterpriseNumber = v6.ClientEnterpriseNumber

const (
	defaultInformationRefreshTime = 4 * time.Hour
	minInformationRefreshTime     = 600 * time.Second // IRT_MINIMUM, required by RFC 9915, Section 21.23.
	maxInformationRefreshTime     = time.Duration(1<<32-1) * time.Second
)

// BackendReader is the interface for getting data from a backend.
type BackendReader interface {
	FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error)
}

// BootURLSource supplies DHCPv6 request parsing options and boot URLs.
// The handler package owns this interface because it consumes the behavior.
type BootURLSource interface {
	InfoOptions() []v6.InfoOption
	BootURL(info v6.Info, hardware *dhcp.Netboot, traceparent string) (string, error)
}

// Config contains the dependencies and behavior used by a Handler.
type Config struct {
	// Backend is required and looks up Hardware by the client's MAC address,
	// including when AutoStatelessEnabled is true.
	Backend BackendReader
	// DNSDefaults supplies fallback DNS servers and domain search domains when
	// Hardware does not provide usable values. Only IPv6 DNS addresses are used.
	DNSDefaults v6.DNSDefaults
	// Log records request handling and errors. Use logr.Discard() to disable logging.
	Log logr.Logger
	// BootURLSource is required and supplies request parsing options and boot URLs.
	// Use v6.DisabledNetboot{} to omit netboot options.
	BootURLSource BootURLSource
	// OTELEnabled appends an available traceparent to iPXE binary filenames,
	// producing <filename>-00-<trace ID>-<span ID>-<trace flags>.
	// It does not control whether request tracing is enabled.
	OTELEnabled bool
	// AutoStatelessEnabled allows replies using defaults when Hardware is not found.
	// It does not bypass other backend errors, unusable Hardware, or disabled DHCP.
	AutoStatelessEnabled bool
	// ServerID is required and identifies this server in replies. Requests carrying
	// a different server ID are ignored.
	ServerID dhcpv6.DUID
	// InformationRefreshTime tells clients when to refresh stateless configuration.
	// New uses four hours for zero or negative values and clamps positive values
	// to the range from 600 to 2^32-1 seconds.
	InformationRefreshTime time.Duration
}

// Handler responds to stateless DHCPv6 requests.
// Its invariants are established by New.
type Handler struct {
	backend                BackendReader
	dnsDefaults            v6.DNSDefaults
	log                    logr.Logger
	bootURLSource          BootURLSource
	otelEnabled            bool
	autoStatelessEnabled   bool
	serverID               dhcpv6.DUID
	informationRefreshTime time.Duration
	infoOptions            []v6.InfoOption
}

// New validates config and constructs a stateless DHCPv6 handler.
func New(config Config) (*Handler, error) {
	if config.Backend == nil {
		return nil, errors.New("DHCPv6 backend is required")
	}
	if config.ServerID == nil {
		return nil, errors.New("DHCPv6 server ID is required")
	}
	if config.BootURLSource == nil {
		return nil, errors.New("DHCPv6 boot URL source is required")
	}

	infoOptions := []v6.InfoOption{v6.WithLogger(config.Log)}
	infoOptions = append(infoOptions, config.BootURLSource.InfoOptions()...)
	infoOptions = append(infoOptions, v6.WithAllowedMessageTypes(dhcpv6.MessageTypeInformationRequest))

	return &Handler{
		backend:                config.Backend,
		dnsDefaults:            config.DNSDefaults,
		log:                    config.Log,
		bootURLSource:          config.BootURLSource,
		otelEnabled:            config.OTELEnabled,
		autoStatelessEnabled:   config.AutoStatelessEnabled,
		serverID:               config.ServerID,
		informationRefreshTime: informationRefreshTime(config.InformationRefreshTime),
		infoOptions:            infoOptions,
	}, nil
}

// Handle responds to DHCPv6 INFORMATION-REQUEST messages with stateless configuration.
func (h *Handler) Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, packet dhcpv6.DHCPv6) {
	// Validate per-call inputs: receiver configuration was validated by New.
	if conn == nil || packet == nil || peer == nil {
		h.log.Error(errors.New("invalid DHCPv6 handler input"), "not able to respond to DHCPv6 packet", "connectionNil", conn == nil, "packetNil", packet == nil, "peerNil", peer == nil)
		return
	}

	tracer := otel.Tracer(tracerName)
	var span trace.Span
	ctx, span = tracer.Start(
		ctx,
		fmt.Sprintf("DHCPv6 Packet Received: %v", packet.Type().String()),
		trace.WithAttributes(v6.EncodeToAttributes(packet, "request", false)...),
		trace.WithAttributes(attribute.String("DHCP.peer", peer.String())),
	)
	defer span.End()

	i, err := v6.NewInfo(peer, packet, h.infoOptions...)
	if err != nil {
		h.log.Info("ignoring DHCPv6 packet: invalid request", "peer", peer.String(), "error", err.Error())
		span.SetStatus(codes.Ok, fmt.Sprintf("ignoring DHCPv6 packet: invalid request: %s", err.Error()))
		return
	}

	log := h.log.WithValues("mac", i.Mac.String(), "xid", i.Msg.TransactionID.String(), "peer", peer.String(), "messageType", i.Msg.Type().String())
	log.Info("received DHCPv6 stateless packet")

	if serverID := i.Msg.Options.ServerID(); serverID != nil && !serverID.Equal(h.serverID) {
		log.Info("ignoring DHCPv6 packet: addressed to another server", "serverID", serverID.String())
		span.SetStatus(codes.Ok, "addressed to another server")
		return
	}

	hw, ok := h.resolveHardware(ctx, i, log, span)
	if !ok {
		return
	}

	// Craft reply message
	reply, err := dhcpv6.NewReplyFromMessage(i.Msg,
		dhcpv6.WithServerID(h.serverID),
		dhcpv6.WithInformationRefreshTime(h.informationRefreshTime),
	)
	if err != nil {
		log.Error(err, "failed to create DHCPv6 reply")
		span.SetStatus(codes.Error, err.Error())
		return
	}

	bootURL, err := h.bootURL(ctx, i, hw)
	if errors.Is(err, v6.ErrNoBootURL) {
		log.Info("bootURL error, DHCPv6 packet: no boot URL available", "error", err.Error())
		span.SetStatus(codes.Ok, "no boot URL available")
		return
	}
	if err != nil {
		log.Error(err, "failed to build DHCPv6 boot URL")
		span.SetStatus(codes.Error, err.Error())
		return
	}
	v6.ApplyBootOptions(reply, i, bootURL)

	v6.ApplyRequestedStatelessOptions(reply, i, hw.DHCP, h.dnsDefaults)

	response, err := v6.WriteReply(conn, peer, i.Relay, reply)
	if err != nil {
		log.Error(err, "failed to send DHCPv6 reply")
		span.SetStatus(codes.Error, err.Error())
		return
	}

	log.Info("sent DHCPv6 stateless reply", "mac", i.Mac.String(), "peer", peer.String(), "messageType", i.Msg.Type().String(), "bootURL", bootURL)
	span.SetAttributes(v6.EncodeToAttributes(response, "reply", false)...)
	span.SetStatus(codes.Ok, "sent DHCPv6 response")
}

func (h *Handler) resolveHardware(ctx context.Context, i v6.Info, log logr.Logger, span trace.Span) (dhcp.Hardware, bool) {
	spec, lookupErr := h.backend.FilterHardware(ctx, data.HardwareFilter{ByMACAddress: i.Mac.String()})
	if lookupErr != nil {
		if v6.HardwareNotFound(lookupErr) {
			if h.autoStatelessEnabled {
				log.Info("DHCPv6 hardware not found, proceeding with auto-stateless defaults", "error", lookupErr.Error())
				return dhcp.Hardware{}, true
			}
			log.Info("ignoring DHCPv6 packet: hardware not found", "mac", i.Mac.String(), "error", lookupErr.Error())
			span.SetStatus(codes.Ok, "hardware not found")
			return dhcp.Hardware{}, false
		}

		log.Error(lookupErr, "ignoring DHCPv6 packet: hardware lookup failed")
		span.SetStatus(codes.Error, lookupErr.Error())
		return dhcp.Hardware{}, false
	}

	hw, err := dhcp.ConvertByMac(ctx, i.Mac, spec)
	if err != nil {
		if h.autoStatelessEnabled {
			log.Error(err, "ignoring DHCPv6 packet: hardware is unusable")
		} else {
			log.Info("ignoring DHCPv6 packet: netboot unavailable", "mac", i.Mac.String(), "error", errString(err))
		}
		span.SetStatus(codes.Error, err.Error())
		return dhcp.Hardware{}, false
	}

	if hw.DHCP != nil && hw.DHCP.Disabled {
		log.Info("ignoring DHCPv6 packet: DHCP is disabled for this MAC address")
		span.SetStatus(codes.Ok, "DHCP is disabled for this MAC address")
		return dhcp.Hardware{}, false
	}

	if hw.DHCP == nil {
		log.Info("ignoring DHCPv6 packet: DHCP unavailable")
		span.SetStatus(codes.Ok, "DHCP unavailable")
		return dhcp.Hardware{}, false
	}

	return hw, true
}

func (h *Handler) bootURL(ctx context.Context, i v6.Info, hw dhcp.Hardware) (string, error) {
	if !i.IsBootfileURLOptionRequested() {
		return "", nil
	}

	var traceparent string
	if h.otelEnabled {
		traceparent = tbotel.TraceparentStringFromContext(ctx)
	}

	bootURL, err := h.bootURLSource.BootURL(i, hw.Netboot, traceparent)
	if err != nil {
		return "", err
	}

	return bootURL, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func informationRefreshTime(configured time.Duration) time.Duration {
	if configured <= 0 {
		return defaultInformationRefreshTime
	}
	if configured < minInformationRefreshTime {
		return minInformationRefreshTime
	}
	if configured > maxInformationRefreshTime {
		return maxInformationRefreshTime
	}
	return configured
}
