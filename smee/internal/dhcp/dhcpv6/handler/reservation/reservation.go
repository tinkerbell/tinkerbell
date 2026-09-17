// Package reservation is the handler for responding to DHCPv6 messages with host reservations.
package reservation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
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

const tracerName = "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6/handler/reservation"

const clientEnterpriseNumber = v6.ClientEnterpriseNumber

const (
	defaultLeaseTime       = 7 * 24 * time.Hour
	minimumDHCPv6LeaseTime = 60 * time.Second
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

// InformationRequestHandler responds to DHCPv6 Information-request messages.
// The reservation handler owns this interface because it delegates that behavior.
type InformationRequestHandler interface {
	Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, packet dhcpv6.DHCPv6)
}

// DerivedConfig enables deterministic IPv6 address assignment.
type DerivedConfig struct {
	// DirectAddressPool supplies the prefix for addresses derived for direct clients.
	// When set, it must be a usable IPv6 unicast prefix between /1 and /64,
	// as checked by v6.UsableDerivedPrefix. The zero value disables direct derivation.
	DirectAddressPool netip.Prefix
	// RelayAddressPrefix is the number of bits preserved from the relay link-address
	// when deriving an address. It must be between 1 and 64; New does not set a default.
	RelayAddressPrefix int
}

// Config contains the dependencies and behavior used by a Handler.
// A nil Derived field selects ordinary reservation behavior.
type Config struct {
	// Backend is required and looks up Hardware by the client's MAC address.
	Backend BackendReader
	// DNSDefaults supplies fallback DNS servers and domain search domains when
	// Hardware does not provide usable values. Only IPv6 DNS addresses are used.
	DNSDefaults v6.DNSDefaults
	// Log records request handling and errors. Use logr.Discard() to disable logging.
	Log logr.Logger
	// BootURLSource is required and supplies request parsing options and boot URLs.
	// Use v6.DisabledNetboot{} to omit netboot options.
	BootURLSource BootURLSource
	// InformationRequestHandler is required and handles stateless requests after
	// Hardware validation, without requiring an IPv6 reservation or derived address.
	// Configure it with the same backend, server ID, and boot settings as this handler.
	InformationRequestHandler InformationRequestHandler
	// OTELEnabled appends an available traceparent to iPXE binary filenames,
	// producing <filename>-00-<trace ID>-<span ID>-<trace flags>.
	// It does not control whether request tracing is enabled.
	OTELEnabled bool
	// Derived enables deterministic address assignment when Hardware has no usable
	// IPv6 reservation. Nil disables derivation. New validates and copies this value.
	Derived *DerivedConfig
	// ServerID is required and identifies this server in replies. Requests carrying
	// a different server ID are ignored.
	ServerID dhcpv6.DUID
}

// Handler responds to DHCPv6 reservation requests.
// Its invariants are established by New.
type Handler struct {
	backend                   BackendReader
	dnsDefaults               v6.DNSDefaults
	log                       logr.Logger
	bootURLSource             BootURLSource
	informationRequestHandler InformationRequestHandler
	otelEnabled               bool
	derived                   *DerivedConfig
	serverID                  dhcpv6.DUID
	infoOptions               []v6.InfoOption
}

// New validates config and constructs a DHCPv6 reservation handler.
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
	if config.InformationRequestHandler == nil {
		return nil, errors.New("DHCPv6 information-request handler is required")
	}

	var derived *DerivedConfig
	if config.Derived != nil {
		if pool := config.Derived.DirectAddressPool; pool.IsValid() && !v6.UsableDerivedPrefix(pool) {
			return nil, fmt.Errorf("invalid DHCPv6 derived direct address pool: %s must be a usable IPv6 unicast prefix with prefix length between /1 and /64", pool)
		}
		if prefix := config.Derived.RelayAddressPrefix; prefix < 1 || prefix > 64 {
			return nil, fmt.Errorf("invalid DHCPv6 derived relay address prefix: %d must be between 1 and 64", prefix)
		}
		configured := *config.Derived
		derived = &configured
	}

	infoOptions := []v6.InfoOption{v6.WithLogger(config.Log)}
	infoOptions = append(infoOptions, config.BootURLSource.InfoOptions()...)
	infoOptions = append(infoOptions, v6.WithAllowedMessageTypes(
		dhcpv6.MessageTypeSolicit,
		dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind,
		dhcpv6.MessageTypeRelease,
		dhcpv6.MessageTypeDecline,
		dhcpv6.MessageTypeInformationRequest,
	))

	return &Handler{
		backend:                   config.Backend,
		dnsDefaults:               config.DNSDefaults,
		log:                       config.Log,
		bootURLSource:             config.BootURLSource,
		informationRequestHandler: config.InformationRequestHandler,
		otelEnabled:               config.OTELEnabled,
		derived:                   derived,
		serverID:                  config.ServerID,
		infoOptions:               infoOptions,
	}, nil
}

func (h *Handler) modeName() string {
	if h.derived != nil {
		return "derived"
	}
	return "reservation"
}

// Handle responds to DHCPv6 reservation messages with reserved IPv6 addresses.
func (h *Handler) Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, packet dhcpv6.DHCPv6) {
	if conn == nil || packet == nil || peer == nil {
		h.log.Error(errors.New("invalid DHCPv6 handler input"), "not able to respond to DHCPv6 packet", "connectionNil", conn == nil, "packetNil", packet == nil, "peerNil", peer == nil)
		return
	}

	tracer := otel.Tracer(tracerName)
	var span trace.Span
	ctx, span = tracer.Start(
		ctx,
		fmt.Sprintf("DHCPv6 Packet Received: %v", packet.Type().String()),
		trace.WithAttributes(v6.EncodeToAttributes(packet, "request", true)...),
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
	log.Info("received DHCPv6 packet", "mode", h.modeName())

	serverID := i.Msg.Options.ServerID()
	if requiresServerID(i.Msg.Type()) && serverID == nil {
		log.Info("ignoring DHCPv6 packet: missing server ID")
		span.SetStatus(codes.Ok, "missing server ID")
		return
	}

	if serverID != nil && !serverID.Equal(h.serverID) {
		log.Info("ignoring DHCPv6 packet: addressed to another server", "serverID", serverID.String())
		span.SetStatus(codes.Ok, "addressed to another server")
		return
	}

	if requiresAddressIA(i.Msg.Type()) && len(i.Msg.Options.IANA()) == 0 {
		log.Info("ignoring DHCPv6 packet: missing IA_NA")
		span.SetStatus(codes.Ok, "missing IA_NA")
		return
	}

	hw, err := h.readBackend(ctx, i.Mac)
	if err != nil {
		if isReleaseOrDecline(i.Msg.Type()) {
			if v6.HardwareNotFound(err) {
				h.writeReleaseOrDeclineReply(conn, peer, i, log, span, noBindingIANAStatuses(i))
				return
			}
			log.Error(err, "ignoring DHCPv6 release/decline: reservation lookup failed", "mode", h.modeName())
			span.SetStatus(codes.Error, err.Error())
			return
		}
		log.Info("ignoring DHCPv6 packet: reservation unavailable", "error", err.Error())
		span.SetStatus(codes.Ok, err.Error())
		return
	}

	if hw.DHCP.Disabled {
		log.Info("DHCP is disabled for this MAC address, no response sent")
		span.SetStatus(codes.Ok, "disabled DHCP response")
		return
	}

	if i.Msg.Type() == dhcpv6.MessageTypeInformationRequest {
		h.informationRequestHandler.Handle(ctx, conn, peer, packet)
		span.SetStatus(codes.Ok, "delegated information-request to stateless handler")
		return
	}

	ipAddress := h.addressFor(i, hw.DHCP.IPAddress)
	if !ipAddress.Is6() {
		if isReleaseOrDecline(i.Msg.Type()) {
			h.writeReleaseOrDeclineReply(conn, peer, i, log, span, noBindingIANAStatuses(i))
			return
		}
		log.Info("ignoring DHCPv6 packet: IPv6 address unavailable", "ipAddress", hw.DHCP.IPAddress.String(), "mode", h.modeName())
		span.SetStatus(codes.Ok, "IPv6 address unavailable")
		return
	}

	if isReleaseOrDecline(i.Msg.Type()) {
		h.writeReleaseOrDeclineReply(conn, peer, i, log, span, releaseOrDeclineIANAStatuses(i, ipAddress))
		return
	}

	reply, err := h.reply(ctx, i, hw.DHCP, hw.Netboot, ipAddress)
	if err != nil {
		if errors.Is(err, v6.ErrNoBootURL) {
			// Netboot requests are all-or-nothing: if a client explicitly asks for
			// Option 59 but we cannot produce a boot URL, do not advertise an address.
			log.Info("bootURL error, DHCPv6 packet: no boot URL available", "mode", h.modeName(), "error", err.Error())
			span.SetStatus(codes.Ok, "no boot URL available")
			return
		}
		log.Error(err, "failed to create DHCPv6 reply", "mode", h.modeName())
		span.SetStatus(codes.Error, err.Error())
		return
	}

	response, err := v6.WriteReply(conn, peer, i.Relay, reply)
	if err != nil {
		log.Error(err, "failed to send DHCPv6 reply")
		span.SetStatus(codes.Error, err.Error())
		return
	}

	log.Info("sent DHCPv6 reply", "ipAddress", ipAddress.String(), "responseType", reply.Type().String(), "mode", h.modeName())
	span.SetAttributes(v6.EncodeToAttributes(response, "reply", true)...)
	span.SetStatus(codes.Ok, "sent DHCPv6 response")
}

func (h *Handler) writeReleaseOrDeclineReply(conn net.PacketConn, peer net.Addr, i v6.Info, log logr.Logger, span trace.Span, ianas []*dhcpv6.OptIANA) {
	reply := &dhcpv6.Message{
		MessageType:   dhcpv6.MessageTypeReply,
		TransactionID: i.Msg.TransactionID,
	}
	cid := i.Msg.GetOneOption(dhcpv6.OptionClientID)
	if cid == nil {
		err := errors.New("client ID cannot be nil when building REPLY")
		log.Error(err, "failed to create DHCPv6 reply", "mode", h.modeName())
		span.SetStatus(codes.Error, err.Error())
		return
	}
	reply.AddOption(cid)
	dhcpv6.WithServerID(h.serverID)(reply)
	dhcpv6.WithOption(&dhcpv6.OptStatusCode{StatusCode: iana.StatusSuccess})(reply)
	for _, ia := range ianas {
		reply.AddOption(ia)
	}

	response, err := v6.WriteReply(conn, peer, i.Relay, reply)
	if err != nil {
		log.Error(err, "failed to send DHCPv6 reply")
		span.SetStatus(codes.Error, err.Error())
		return
	}

	log.Info("sent DHCPv6 reply", "responseType", reply.Type().String(), "mode", h.modeName())
	span.SetAttributes(v6.EncodeToAttributes(response, "reply", true)...)
	span.SetStatus(codes.Ok, "sent DHCPv6 response")
}

func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (dhcp.Hardware, error) {
	spec, err := h.backend.FilterHardware(ctx, data.HardwareFilter{ByMACAddress: mac.String()})
	if err != nil {
		return dhcp.Hardware{}, err
	}

	hw, err := dhcp.ConvertByMac(ctx, mac, spec)
	if err != nil {
		return dhcp.Hardware{}, err
	}

	if hw.DHCP == nil {
		return dhcp.Hardware{}, errors.New("no DHCP data")
	}

	return hw, nil
}

func (h *Handler) reply(ctx context.Context, i v6.Info, d *dhcp.DHCP, n *dhcp.Netboot, ipAddress netip.Addr) (*dhcpv6.Message, error) {
	bootURL, err := h.bootURL(ctx, i, d, n)
	if err != nil {
		return nil, err
	}

	mods := []dhcpv6.Modifier{
		dhcpv6.WithServerID(h.serverID),
		h.withAddressOptions(i, d, ipAddress, bootURL),
	}

	switch i.Msg.Type() {
	case dhcpv6.MessageTypeSolicit:
		if i.Msg.GetOneOption(dhcpv6.OptionRapidCommit) != nil {
			return dhcpv6.NewReplyFromMessage(i.Msg, mods...)
		}
		return dhcpv6.NewAdvertiseFromSolicit(i.Msg, mods...)
	case dhcpv6.MessageTypeRequest, dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind:
		return dhcpv6.NewReplyFromMessage(i.Msg, mods...)
	default:
		return nil, errors.New("unsupported DHCPv6 message type")
	}
}

func (h *Handler) bootURL(ctx context.Context, i v6.Info, _ *dhcp.DHCP, n *dhcp.Netboot) (string, error) {
	if !i.IsBootfileURLOptionRequested() {
		return "", nil
	}

	var traceparent string
	if h.otelEnabled {
		traceparent = tbotel.TraceparentStringFromContext(ctx)
	}

	bootURL, err := h.bootURLSource.BootURL(i, n, traceparent)
	if err != nil {
		return "", err
	}

	return bootURL, nil
}

func (h *Handler) withAddressOptions(i v6.Info, d *dhcp.DHCP, ipAddress netip.Addr, bootURL string) dhcpv6.Modifier {
	return func(reply dhcpv6.DHCPv6) {
		if msg, ok := reply.(*dhcpv6.Message); ok {
			for _, ia := range h.ianaOptions(i, d, ipAddress) {
				msg.AddOption(ia)
			}
		}

		v6.ApplyBootOptions(reply, i, bootURL)
		v6.ApplyRequestedStatelessOptions(reply, i, d, h.dnsDefaults)
	}
}

func (h *Handler) ianaOptions(i v6.Info, d *dhcp.DHCP, ipAddress netip.Addr) []*dhcpv6.OptIANA {
	if requiresExistingBinding(i.Msg.Type()) {
		ias := i.Msg.Options.IANA()
		options := make([]*dhcpv6.OptIANA, 0, len(ias))
		assigned := false
		unassignedStatus := iana.StatusNoBinding
		if i.Msg.Type() == dhcpv6.MessageTypeRequest {
			unassignedStatus = iana.StatusNoAddrsAvail
		}
		for _, ia := range ias {
			if !assigned && requestedIAAcceptable(i.Msg.Type(), ia, ipAddress) {
				options = append(options, h.ianaWithIAID(ia.IaId, d, ipAddress))
				assigned = true
			} else {
				options = append(options, ianaStatus(ia.IaId, unassignedStatus))
			}
		}
		return options
	}

	if i.Msg.Type() == dhcpv6.MessageTypeSolicit {
		ias := i.Msg.Options.IANA()
		options := make([]*dhcpv6.OptIANA, 0, len(ias))
		for idx, ia := range ias {
			if idx == 0 {
				options = append(options, h.ianaWithIAID(ia.IaId, d, ipAddress))
				continue
			}
			options = append(options, ianaStatus(ia.IaId, iana.StatusNoAddrsAvail))
		}
		return options
	}

	return nil
}

func (h *Handler) ianaWithIAID(iaid [4]byte, d *dhcp.DHCP, ipAddress netip.Addr) *dhcpv6.OptIANA {
	t1, t2, preferredLifetime, validLifetime := statefulLifetimes(d.LeaseTime)

	return &dhcpv6.OptIANA{
		IaId: iaid,
		T1:   t1,
		T2:   t2,
		Options: dhcpv6.IdentityOptions{Options: []dhcpv6.Option{
			&dhcpv6.OptIAAddress{
				IPv6Addr:          net.IP(ipAddress.AsSlice()),
				PreferredLifetime: preferredLifetime,
				ValidLifetime:     validLifetime,
			},
		}},
	}
}

func statefulLifetimes(leaseTime uint32) (t1, t2, preferredLifetime, validLifetime time.Duration) {
	validLifetime = time.Duration(leaseTime) * time.Second
	if leaseTime == 0 {
		validLifetime = defaultLeaseTime
	}
	if validLifetime < minimumDHCPv6LeaseTime {
		validLifetime = minimumDHCPv6LeaseTime
	}

	// DHCPv6 lifetimes use whole seconds. Base renewal and rebinding on the
	// advertised preferred lifetime, per RFC 9915 section 21.4, so both begin
	// before the address is deprecated.
	preferredLifetime = (validLifetime / 2).Truncate(time.Second)
	t1 = (preferredLifetime / 2).Truncate(time.Second)
	t2 = (preferredLifetime * 4 / 5).Truncate(time.Second)

	return t1, t2, preferredLifetime, validLifetime
}

func (h *Handler) addressFor(i v6.Info, reservation netip.Addr) netip.Addr {
	if usableReservationAddress(reservation) {
		return reservation
	}
	if h.derived == nil {
		return netip.Addr{}
	}
	if i.Relay != nil {
		linkAddr, ok := relayLinkAddress(i.Relay)
		if !ok || !usableRelayLinkAddress(linkAddr) {
			return netip.Addr{}
		}
		return derivedAddress(netip.PrefixFrom(linkAddr, h.derived.RelayAddressPrefix), i.Mac)
	}
	return derivedAddress(h.derived.DirectAddressPool, i.Mac)
}

func usableReservationAddress(addr netip.Addr) bool {
	return addr.Is6() &&
		!addr.Is4In6() &&
		!addr.IsUnspecified() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLoopback() &&
		!addr.IsMulticast()
}

func usableRelayLinkAddress(addr netip.Addr) bool {
	return addr.Is6() &&
		!addr.Is4In6() &&
		!addr.IsUnspecified() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLoopback() &&
		!addr.IsMulticast()
}

func relayLinkAddress(relay *dhcpv6.RelayMessage) (netip.Addr, bool) {
	innerMost, err := dhcpv6.DecapsulateRelayIndex(relay, -1)
	if err != nil {
		return netip.Addr{}, false
	}
	relayMsg, ok := innerMost.(*dhcpv6.RelayMessage)
	if !ok {
		return netip.Addr{}, false
	}
	return netip.AddrFromSlice(relayMsg.LinkAddr)
}

func derivedAddress(pool netip.Prefix, mac net.HardwareAddr) netip.Addr {
	if !v6.UsableDerivedPrefix(pool) || len(mac) == 0 {
		return netip.Addr{}
	}

	hash := sha256.Sum256([]byte(mac.String()))
	return derivedAddressFromHash(pool, hash)
}

// derivedAddressFromHash keeps the network bits from pool and fills only the
// host bits with hash bits, starting from the least-significant end of the
// address. This makes the same MAC hash stable inside a pool while ensuring the
// result never escapes the configured prefix.
func derivedAddressFromHash(pool netip.Prefix, hash [32]byte) netip.Addr {
	addr := pool.Masked().Addr().As16()
	hostBits := 128 - pool.Bits()
	for bit := range hostBits {
		hashBit := (hash[len(hash)-1-(bit/8)] >> (bit % 8)) & 1
		byteIndex := 15 - (bit / 8)
		bitMask := byte(1 << (bit % 8))
		if hashBit == 1 {
			addr[byteIndex] |= bitMask
		} else {
			addr[byteIndex] &^= bitMask
		}
	}

	// Reserve the subnet-router anycast address (all host bits zero). If the
	// hash lands there, move to the first ordinary host address in the prefix.
	if netip.AddrFrom16(addr) == pool.Masked().Addr() {
		addr[15] |= 1
	}

	return netip.AddrFrom16(addr)
}

func ianaStatus(iaid [4]byte, status iana.StatusCode) *dhcpv6.OptIANA {
	return &dhcpv6.OptIANA{
		IaId: iaid,
		Options: dhcpv6.IdentityOptions{Options: []dhcpv6.Option{
			&dhcpv6.OptStatusCode{StatusCode: status},
		}},
	}
}

func iaid(i v6.Info) [4]byte {
	if ia := i.Msg.Options.OneIANA(); ia != nil {
		return ia.IaId
	}
	return [4]byte{}
}

func isReleaseOrDecline(messageType dhcpv6.MessageType) bool {
	return messageType == dhcpv6.MessageTypeRelease || messageType == dhcpv6.MessageTypeDecline
}

func requiresExistingBinding(messageType dhcpv6.MessageType) bool {
	return messageType == dhcpv6.MessageTypeRequest ||
		messageType == dhcpv6.MessageTypeRenew ||
		messageType == dhcpv6.MessageTypeRebind
}

func requiresAddressIA(messageType dhcpv6.MessageType) bool {
	return messageType == dhcpv6.MessageTypeSolicit ||
		requiresExistingBinding(messageType)
}

func requiresServerID(messageType dhcpv6.MessageType) bool {
	switch messageType {
	case dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRelease,
		dhcpv6.MessageTypeDecline:
		return true
	default:
		return false
	}
}

func requestedIAMatches(ia *dhcpv6.OptIANA, reservation netip.Addr) bool {
	if ia == nil || !reservation.IsValid() {
		return false
	}

	addresses := ia.Options.Addresses()
	if len(addresses) != 1 || addresses[0] == nil {
		return false
	}

	requested, ok := netip.AddrFromSlice(addresses[0].IPv6Addr)
	return ok && requested == reservation
}

func requestedIAAcceptable(messageType dhcpv6.MessageType, ia *dhcpv6.OptIANA, reservation netip.Addr) bool {
	if requestedIAMatches(ia, reservation) {
		return true
	}
	if messageType != dhcpv6.MessageTypeRequest || ia == nil || !reservation.IsValid() {
		return false
	}

	return len(ia.Options.Addresses()) == 0
}

func releaseOrDeclineIANAStatuses(i v6.Info, reservation netip.Addr) []*dhcpv6.OptIANA {
	ias := i.Msg.Options.IANA()
	if len(ias) == 0 {
		return []*dhcpv6.OptIANA{ianaStatus(iaid(i), iana.StatusNoBinding)}
	}

	statuses := make([]*dhcpv6.OptIANA, 0, len(ias))
	for _, ia := range ias {
		status := iana.StatusNoBinding
		if requestedIAMatches(ia, reservation) {
			status = iana.StatusSuccess
		}
		statuses = append(statuses, ianaStatus(ia.IaId, status))
	}
	return statuses
}

func noBindingIANAStatuses(i v6.Info) []*dhcpv6.OptIANA {
	ias := i.Msg.Options.IANA()
	if len(ias) == 0 {
		return []*dhcpv6.OptIANA{ianaStatus(iaid(i), iana.StatusNoBinding)}
	}

	statuses := make([]*dhcpv6.OptIANA, 0, len(ias))
	for _, ia := range ias {
		statuses = append(statuses, ianaStatus(ia.IaId, iana.StatusNoBinding))
	}
	return statuses
}
