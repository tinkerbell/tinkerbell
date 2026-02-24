package data

import (
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.opentelemetry.io/otel/attribute"
)

// Hardware is the combination of structs that hold all the data about a piece of hardware.
type Hardware struct {
	// Holds the assigned AgentID from the hardware object
	AgentID string
	// DHCP holds the DHCP headers and options to be set in a DHCP handler response.
	// This is the API between a DHCP handler and a backend.
	DHCP *DHCP
	// Netboot holds info used in netbooting a client.
	Netboot *Netboot
	// Isoboot holds info used in booting a client using an ISO image.
	Isoboot *Isoboot
}

// DHCP holds the DHCP headers and options to be set in a DHCP handler response.
// This is the API between a DHCP handler and a backend.
type DHCP struct {
	MACAddress            net.HardwareAddr // chaddr DHCP header.
	IPAddress             netip.Addr       // yiaddr DHCP header.
	SubnetMask            net.IPMask       // DHCP option 1.
	DefaultGateway        netip.Addr       // DHCP option 3.
	NameServers           []net.IP         // DHCP option 6.
	Hostname              string           // DHCP option 12.
	DomainName            string           // DHCP option 15.
	BroadcastAddress      netip.Addr       // DHCP option 28.
	NTPServers            []net.IP         // DHCP option 42.
	VLANID                string           // DHCP option 43.116.
	LeaseTime             uint32           // DHCP option 51.
	TFTPServerName        string           // DHCP option 66.
	BootFileName          string           // DHCP option 67.
	Arch                  string           // DHCP option 93.
	DomainSearch          []string         // DHCP option 119.
	ClasslessStaticRoutes dhcpv4.Routes    // DHCP option 121 - RFC 3442.
	Disabled              bool             // If true, no DHCP response should be sent.
}

// Netboot holds info used in netbooting a client.
type Netboot struct {
	AllowNetboot  bool     // If true, the client will be provided netboot options in the DHCP offer/ack.
	IPXEScriptURL *url.URL // Overrides a default value that is passed into DHCP on startup.
	IPXEScript    string   // Overrides a default value that is passed into DHCP on startup.
	IPXEBinary    string   // Overrides Smee's default architecture to binary mapping.
	Console       string
	Facility      string
	OSIE          OSIE
}

// Isoboot holds info used in booting a client using an ISO image.
type Isoboot struct {
	// SourceISO is the source url where HookOS, an Operating System Installation Environment (OSIE), ISO lives.
	// It must be a valid url.URL{} object and must have a url.URL{}.Scheme of HTTP or HTTPS.
	//+optional
	SourceISO *url.URL
}

// OSIE or OS Installation Environment is the data about where the OSIE parts are located.
type OSIE struct {
	// BaseURL is the URL where the OSIE parts are located.
	BaseURL *url.URL
	// Kernel is the name of the kernel file.
	Kernel string
	// Initrd is the name of the initrd file.
	Initrd string
}

// EncodeToAttributes returns a slice of opentelemetry attributes that can be used to set span.SetAttributes.
func (d *DHCP) EncodeToAttributes() []attribute.KeyValue {
	var ns []string
	for _, e := range d.NameServers {
		ns = append(ns, e.String())
	}

	var ntp []string
	for _, e := range d.NTPServers {
		ntp = append(ntp, e.String())
	}

	var ip string
	if d.IPAddress.Compare(netip.Addr{}) != 0 {
		ip = d.IPAddress.String()
	}

	var sm string
	if d.SubnetMask != nil {
		sm = net.IP(d.SubnetMask).String()
	}

	var dfg string
	if d.DefaultGateway.Compare(netip.Addr{}) != 0 {
		dfg = d.DefaultGateway.String()
	}

	var ba string
	if d.BroadcastAddress.Compare(netip.Addr{}) != 0 {
		ba = d.BroadcastAddress.String()
	}

	var routes []string
	for _, route := range d.ClasslessStaticRoutes {
		routes = append(routes, route.Dest.String()+"->"+route.Router.String())
	}

	return []attribute.KeyValue{
		attribute.String("DHCP.MACAddress", d.MACAddress.String()),
		attribute.String("DHCP.IPAddress", ip),
		attribute.String("DHCP.SubnetMask", sm),
		attribute.String("DHCP.DefaultGateway", dfg),
		attribute.String("DHCP.NameServers", strings.Join(ns, ",")),
		attribute.String("DHCP.Hostname", d.Hostname),
		attribute.String("DHCP.DomainName", d.DomainName),
		attribute.String("DHCP.BroadcastAddress", ba),
		attribute.String("DHCP.NTPServers", strings.Join(ntp, ",")),
		attribute.Int64("DHCP.LeaseTime", int64(d.LeaseTime)),
		attribute.String("DHCP.DomainSearch", strings.Join(d.DomainSearch, ",")),
		attribute.String("DHCP.ClasslessStaticRoutes", strings.Join(routes, ",")),
	}
}

// EncodeToAttributes returns a slice of opentelemetry attributes that can be used to set span.SetAttributes.
func (n *Netboot) EncodeToAttributes() []attribute.KeyValue {
	a := []attribute.KeyValue{
		attribute.Bool("Netboot.AllowNetboot", n.AllowNetboot),
	}
	if n.IPXEScriptURL != nil {
		a = append(a, attribute.String("Netboot.IPXEScriptURL", n.IPXEScriptURL.String()))
	}
	if n.IPXEBinary != "" {
		a = append(a, attribute.String("Netboot.IPXEBinary", n.IPXEBinary))
	}
	return a
}
