// Package reservation is the handler for responding to DHCPv6 messages with host reservations.
package reservation

import (
	"context"
	"crypto/sha256"
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
	statelessv6 "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6/handler/stateless"
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

// Handler holds the configuration details for the running DHCPv6 server.
type Handler struct {
	// Backend is the backend to use for getting DHCP data.
	Backend BackendReader

	// Log is used to log messages.
	// `logr.Discard()` can be used if no logging is desired.
	Log logr.Logger

	// Netboot configuration.
	Netboot Netboot

	// OTELEnabled is used to determine if netboot options include otel naming.
	// When true, the netboot filename will be appended with otel information.
	// For example, the filename will be "snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01".
	// <original filename>-00-<trace id>-<span id>-<trace flags>
	OTELEnabled bool

	// Derived enables deterministic IPv6 address assignment when no backend address reservation exists.
	Derived bool
	// DerivedDirectAddressPool is the IPv6 prefix used to derive addresses for direct client requests.
	DerivedDirectAddressPool netip.Prefix
	// DerivedRelayAddressPrefix controls how many relay link-address prefix bits are preserved in derived addresses.
	DerivedRelayAddressPrefix int

	// ServerID is the DHCPv6 server identifier included in replies and matched against client requests.
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

func (h *Handler) modeName() string {
	if h.Derived {
		return "derived"
	}
	return "reservation"
}

// Handle responds to DHCPv6 reservation messages with reserved IPv6 addresses.
func (h *Handler) Handle(ctx context.Context, conn net.PacketConn, peer net.Addr, packet dhcpv6.DHCPv6) {
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
		trace.WithAttributes(v6.EncodeToAttributes(packet, "request", true)...),
		trace.WithAttributes(attribute.String("DHCP.peer", peer.String())),
	)
	defer span.End()

	i, err := v6.NewInfo(
		peer,
		packet,
		v6.WithLogger(h.Log),
		v6.WithMacAddrFormat(h.Netboot.InjectMacAddrFormat),
		v6.WithArchMappingOverride(h.Netboot.IPXEArchMapping),
		v6.WithAllowedMessageTypes(
			dhcpv6.MessageTypeSolicit,
			dhcpv6.MessageTypeRequest,
			dhcpv6.MessageTypeRenew,
			dhcpv6.MessageTypeRebind,
			dhcpv6.MessageTypeRelease,
			dhcpv6.MessageTypeDecline,
			dhcpv6.MessageTypeInformationRequest,
		),
	)
	if err != nil {
		h.Log.Info("ignoring DHCPv6 packet: invalid request", "peer", peer.String(), "error", err.Error())
		span.SetStatus(codes.Ok, fmt.Sprintf("ignoring DHCPv6 packet: invalid request: %s", err.Error()))
		return
	}

	log := h.Log.WithValues("mac", i.Mac.String(), "xid", i.Msg.TransactionID.String(), "peer", peer.String(), "messageType", i.Msg.Type().String())
	log.Info("received DHCPv6 packet", "mode", h.modeName())

	serverID := i.Msg.Options.ServerID()
	if requiresServerID(i.Msg.Type()) && serverID == nil {
		log.Info("ignoring DHCPv6 packet: missing server ID")
		span.SetStatus(codes.Ok, "missing server ID")
		return
	}

	if serverID != nil && !serverID.Equal(h.ServerID) {
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

	if i.Msg.Type() == dhcpv6.MessageTypeInformationRequest {
		h.statelessHandler().Handle(ctx, conn, peer, packet)
		span.SetStatus(codes.Ok, "delegated information-request to stateless handler")
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
	dhcpv6.WithServerID(h.ServerID)(reply)
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

func (h *Handler) statelessHandler() *statelessv6.Handler {
	return &statelessv6.Handler{
		Backend: h.Backend,
		Log:     h.Log,
		Netboot: statelessv6.Netboot{
			IPXEBinServerTFTP:   h.Netboot.IPXEBinServerTFTP,
			IPXEBinServerHTTP:   h.Netboot.IPXEBinServerHTTP,
			IPXEScriptURL:       h.Netboot.IPXEScriptURL,
			Enabled:             h.Netboot.Enabled,
			InjectMacAddrFormat: h.Netboot.InjectMacAddrFormat,
			IPXEArchMapping:     h.Netboot.IPXEArchMapping,
		},
		OTELEnabled:            h.OTELEnabled,
		ServerID:               h.ServerID,
		InformationRefreshTime: h.InformationRefreshTime,
	}
}

func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (dhcp.Hardware, error) {
	spec, err := h.Backend.FilterHardware(ctx, data.HardwareFilter{ByMACAddress: mac.String()})
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
		dhcpv6.WithServerID(h.ServerID),
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
	if !h.Netboot.Enabled || !i.IsBootfileURLOptionRequested() {
		return "", nil
	}

	var traceparent string
	if h.OTELEnabled {
		traceparent = tbotel.TraceparentStringFromContext(ctx)
	}

	bootURL, err := i.BootURL(v6.BootURLConfig{
		HardwareNetboot:   n,
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

func (h *Handler) withAddressOptions(i v6.Info, d *dhcp.DHCP, ipAddress netip.Addr, bootURL string) dhcpv6.Modifier {
	return func(reply dhcpv6.DHCPv6) {
		if msg, ok := reply.(*dhcpv6.Message); ok {
			for _, ia := range h.ianaOptions(i, d, ipAddress) {
				msg.AddOption(ia)
			}
		}

		v6.ApplyBootOptions(reply, i, bootURL)
		v6.ApplyRequestedStatelessOptions(reply, i, d)
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

	preferredLifetime = validLifetime / 2
	t1 = validLifetime / 2
	t2 = validLifetime * 4 / 5

	return t1, t2, preferredLifetime, validLifetime
}

func (h *Handler) addressFor(i v6.Info, reservation netip.Addr) netip.Addr {
	if usableReservationAddress(reservation) {
		return reservation
	}
	if !h.Derived {
		return netip.Addr{}
	}
	if i.Relay != nil {
		linkAddr, ok := relayLinkAddress(i.Relay)
		if !ok || !usableRelayLinkAddress(linkAddr) {
			return netip.Addr{}
		}
		return derivedAddress(netip.PrefixFrom(linkAddr, h.DerivedRelayAddressPrefix), i.Mac)
	}
	return derivedAddress(h.DerivedDirectAddressPool, i.Mac)
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
