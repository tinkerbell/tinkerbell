package stateless

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
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
	maxInformationRefreshTime     = time.Duration(1<<32-1) * time.Second
)

// BackendReader is the interface for getting data from a backend.
type BackendReader interface {
	FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error)
}

// Handler holds the configuration details for the running DHCPv6 server.
type Handler struct {
	// Backend is the backend to use for getting DHCP data.
	Backend BackendReader

	// Log is used to log messages.
	// `logr.Discard()` can be used if no logging is desired.
	Log logr.Logger

	// Netboot configuration
	Netboot Netboot

	// OTELEnabled is used to determine if netboot options include otel naming.
	// When true, the netboot filename will be appended with otel information.
	// For example, the filename will be "snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01".
	// <original filename>-00-<trace id>-<span id>-<trace flags>
	OTELEnabled bool

	// AutoStatelessEnabled allows replies for unknown hardware, using default
	// boot settings when the client requests them.
	AutoStatelessEnabled bool

	// ServerID is the DHCPv6 server identifier included in replies.
	ServerID dhcpv6.DUID
	// InformationRefreshTime controls how long clients wait before refreshing stateless configuration.
	InformationRefreshTime time.Duration
}

// Netboot holds the netboot configuration details used in running a DHCP server.
type Netboot struct {
	// iPXE binary server IP:Port serving via TFTP.
	IPXEBinServerTFTP netip.AddrPort

	// IPXEBinServerHTTP is the URL to the IPXE binary server serving via HTTP(s).
	IPXEBinServerHTTP *url.URL

	// IPXEScriptURL is the URL to the IPXE script to use.
	IPXEScriptURL func(net.HardwareAddr) *url.URL

	// Enabled is whether to enable sending netboot DHCP options.
	Enabled bool

	// InjectMacAddrFormat is the format to use when injecting the mac address into the iPXE binary URL.
	InjectMacAddrFormat constant.MACFormat

	// IPXEArchMapping will override the default architecture to binary mapping.
	IPXEArchMapping map[iana.Arch]constant.IPXEBinary
}

// Handle responds to DHCPv6 INFORMATION-REQUEST messages with stateless configuration.
func (h *Handler) Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, packet dhcpv6.DHCPv6) {
	// Validations
	if conn == nil || packet == nil || peer == nil {
		h.Log.Error(errors.New("invalid DHCPv6 handler input"), "not able to respond to DHCPv6 packet", "connectionNil", conn == nil, "packetNil", packet == nil, "peerNil", peer == nil)
		return
	}
	if h.Backend == nil || h.ServerID == nil {
		h.Log.Error(errors.New("invalid DHCPv6 handler configuration"), "not able to respond to DHCPv6 packet", "backendNil", h.Backend == nil, "serverIDNil", h.ServerID == nil)
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

	informationRefreshTime := informationRefreshTime(h.InformationRefreshTime)

	i, err := v6.NewInfo(
		peer,
		packet,
		v6.WithLogger(h.Log),
		v6.WithMacAddrFormat(h.Netboot.InjectMacAddrFormat),
		v6.WithArchMappingOverride(h.Netboot.IPXEArchMapping),
		v6.WithAllowedMessageTypes(
			dhcpv6.MessageTypeInformationRequest,
		),
	)
	if err != nil {
		h.Log.Info("ignoring DHCPv6 packet: invalid request", "peer", peer.String(), "error", err.Error())
		span.SetStatus(codes.Ok, fmt.Sprintf("ignoring DHCPv6 packet: invalid request: %s", err.Error()))
		return
	}

	log := h.Log.WithValues("mac", i.Mac.String(), "xid", i.Msg.TransactionID.String(), "peer", peer.String(), "messageType", i.Msg.Type().String())
	log.Info("received DHCPv6 stateless packet")

	if serverID := i.Msg.Options.ServerID(); serverID != nil && !serverID.Equal(h.ServerID) {
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
		dhcpv6.WithServerID(h.ServerID),
		dhcpv6.WithInformationRefreshTime(informationRefreshTime),
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

	if hw.DHCP != nil {
		v6.ApplyRequestedStatelessOptions(reply, i, hw.DHCP)
	}

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
	spec, lookupErr := h.Backend.FilterHardware(ctx, data.HardwareFilter{ByMACAddress: i.Mac.String()})
	if lookupErr != nil {
		if v6.HardwareNotFound(lookupErr) {
			if h.AutoStatelessEnabled {
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
		if h.AutoStatelessEnabled {
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
	if !h.Netboot.Enabled || !i.IsBootfileURLOptionRequested() {
		return "", nil
	}

	var traceparent string
	if h.OTELEnabled {
		traceparent = tbotel.TraceparentStringFromContext(ctx)
	}

	bootURL, err := i.BootURL(v6.BootURLConfig{
		HardwareNetboot:   hw.Netboot,
		IPXEBinServerTFTP: h.Netboot.IPXEBinServerTFTP,
		IPXEBinServerHTTP: h.Netboot.IPXEBinServerHTTP,
		IPXEScriptURL:     h.Netboot.IPXEScriptURL,
		Traceparent:       traceparent,
	})
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
	if configured > maxInformationRefreshTime {
		return maxInformationRefreshTime
	}
	return configured
}
