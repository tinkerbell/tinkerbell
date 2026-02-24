package data

import (
	"net"
	"net/netip"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.opentelemetry.io/otel/attribute"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// mustParseCIDR is a helper function for tests to parse CIDR strings
func mustParseCIDR(cidr string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return ipnet
}

func TestDHCPEncodeToAttributes(t *testing.T) {
	tests := map[string]struct {
		dhcp *DHCP
		want []attribute.KeyValue
	}{
		"successful encode of zero value DHCP struct": {
			dhcp: &DHCP{},
			want: []attribute.KeyValue{
				attribute.String("DHCP.MACAddress", ""),
				attribute.String("DHCP.IPAddress", ""),
				attribute.String("DHCP.Hostname", ""),
				attribute.String("DHCP.SubnetMask", ""),
				attribute.String("DHCP.DefaultGateway", ""),
				attribute.String("DHCP.NameServers", ""),
				attribute.String("DHCP.DomainName", ""),
				attribute.String("DHCP.BroadcastAddress", ""),
				attribute.String("DHCP.NTPServers", ""),
				attribute.Int64("DHCP.LeaseTime", 0),
				attribute.String("DHCP.DomainSearch", ""),
				attribute.String("DHCP.ClasslessStaticRoutes", ""),
			},
		},
		"successful encode of populated DHCP struct": {
			dhcp: &DHCP{
				MACAddress:       []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
				IPAddress:        netip.MustParseAddr("192.168.2.150"),
				SubnetMask:       []byte{255, 255, 255, 0},
				DefaultGateway:   netip.MustParseAddr("192.168.2.1"),
				NameServers:      []net.IP{{1, 1, 1, 1}, {8, 8, 8, 8}},
				Hostname:         "test",
				DomainName:       "example.com",
				BroadcastAddress: netip.MustParseAddr("192.168.2.255"),
				NTPServers:       []net.IP{{132, 163, 96, 2}},
				LeaseTime:        86400,
				DomainSearch:     []string{"example.com", "example.org"},
				ClasslessStaticRoutes: dhcpv4.Routes{
					&dhcpv4.Route{
						Dest:   mustParseCIDR("10.0.0.0/8"),
						Router: netip.MustParseAddr("192.168.2.10").AsSlice(),
					},
				},
			},
			want: []attribute.KeyValue{
				attribute.String("DHCP.MACAddress", "00:01:02:03:04:05"),
				attribute.String("DHCP.IPAddress", "192.168.2.150"),
				attribute.String("DHCP.Hostname", "test"),
				attribute.String("DHCP.SubnetMask", "255.255.255.0"),
				attribute.String("DHCP.DefaultGateway", "192.168.2.1"),
				attribute.String("DHCP.NameServers", "1.1.1.1,8.8.8.8"),
				attribute.String("DHCP.DomainName", "example.com"),
				attribute.String("DHCP.BroadcastAddress", "192.168.2.255"),
				attribute.String("DHCP.NTPServers", "132.163.96.2"),
				attribute.Int64("DHCP.LeaseTime", 86400),
				attribute.String("DHCP.DomainSearch", "example.com,example.org"),
				attribute.String("DHCP.ClasslessStaticRoutes", "10.0.0.0/8->192.168.2.10"),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			want := attribute.NewSet(tt.want...)
			got := attribute.NewSet(tt.dhcp.EncodeToAttributes()...)
			enc := attribute.DefaultEncoder()
			if diff := cmp.Diff(got.Encoded(enc), want.Encoded(enc)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestNetbootEncodeToAttributes(t *testing.T) {
	tests := map[string]struct {
		netboot *Netboot
		want    []attribute.KeyValue
	}{
		"successful encode of zero value Netboot struct": {
			netboot: &Netboot{},
			want: []attribute.KeyValue{
				attribute.Bool("Netboot.AllowNetboot", false),
			},
		},
		"successful encode of populated Netboot struct": {
			netboot: &Netboot{
				AllowNetboot:  true,
				IPXEScriptURL: &url.URL{Scheme: "http", Host: "example.com"},
			},
			want: []attribute.KeyValue{
				attribute.Bool("Netboot.AllowNetboot", true),
				attribute.String("Netboot.IPXEScriptURL", "http://example.com"),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			want := attribute.NewSet(tt.want...)
			got := attribute.NewSet(tt.netboot.EncodeToAttributes()...)
			enc := attribute.DefaultEncoder()
			if diff := cmp.Diff(got.Encoded(enc), want.Encoded(enc)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestToDHCPData(t *testing.T) {
	tests := map[string]struct {
		in        *tinkerbell.DHCP
		want      *DHCP
		shouldErr bool
	}{
		"nil input": {
			in:        nil,
			shouldErr: true,
		},
		"bad mac": {
			in:        &tinkerbell.DHCP{MAC: "bad"},
			shouldErr: true,
		},
		"no ip": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{}},
			shouldErr: true,
		},
		"no subnet": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{Address: "192.168.2.4"}},
			shouldErr: true,
		},
		"bad IP": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{Address: "bad"}},
			shouldErr: true,
		},
		"bad gateway": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{Address: "192.168.2.4", Netmask: "255.255.254.0", Gateway: "bad"}},
			shouldErr: true,
		},
		"one bad nameserver": {
			in: &tinkerbell.DHCP{
				MAC:         "00:00:00:00:00:04",
				NameServers: []string{"1.1.1.1", "bad"},
				IP: &tinkerbell.IP{
					Address: "192.168.2.4",
					Netmask: "255.255.0.0",
					Gateway: "192.168.2.1",
				},
			},
			want: &DHCP{
				SubnetMask:     net.IPv4Mask(255, 255, 0, 0),
				DefaultGateway: netip.MustParseAddr("192.168.2.1"),
				NameServers:    []net.IP{net.IPv4(1, 1, 1, 1)},
				IPAddress:      netip.MustParseAddr("192.168.2.4"),
				MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x04},
			},
		},
		"full": {
			in: &tinkerbell.DHCP{
				MAC:         "00:00:00:00:00:04",
				Hostname:    "test",
				LeaseTime:   3600,
				NameServers: []string{"1.1.1.1"},
				IP: &tinkerbell.IP{
					Address: "192.168.1.4",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
				},
			},
			want: &DHCP{
				SubnetMask:     net.IPv4Mask(255, 255, 255, 0),
				DefaultGateway: netip.MustParseAddr("192.168.1.1"),
				NameServers:    []net.IP{net.IPv4(1, 1, 1, 1)},
				Hostname:       "test",
				LeaseTime:      3600,
				IPAddress:      netip.MustParseAddr("192.168.1.4"),
				MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x04},
			},
		},
		"with classless static routes": {
			in: &tinkerbell.DHCP{
				MAC:         "00:00:00:00:00:05",
				Hostname:    "test-with-routes",
				LeaseTime:   7200,
				NameServers: []string{"8.8.8.8"},
				IP: &tinkerbell.IP{
					Address: "192.168.2.5",
					Netmask: "255.255.255.0",
					Gateway: "192.168.2.1",
				},
				ClasslessStaticRoutes: []tinkerbell.ClasslessStaticRoute{
					{
						DestinationDescriptor: "10.0.0.0/8",
						Router:                "192.168.2.10",
					},
					{
						DestinationDescriptor: "172.16.0.0/12",
						Router:                "192.168.2.20",
					},
				},
			},
			want: &DHCP{
				SubnetMask:     net.IPv4Mask(255, 255, 255, 0),
				DefaultGateway: netip.MustParseAddr("192.168.2.1"),
				NameServers:    []net.IP{net.IPv4(8, 8, 8, 8)},
				Hostname:       "test-with-routes",
				LeaseTime:      7200,
				IPAddress:      netip.MustParseAddr("192.168.2.5"),
				MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x05},
				ClasslessStaticRoutes: dhcpv4.Routes{
					&dhcpv4.Route{
						Dest:   mustParseCIDR("10.0.0.0/8"),
						Router: netip.MustParseAddr("192.168.2.10").AsSlice(),
					},
					&dhcpv4.Route{
						Dest:   mustParseCIDR("172.16.0.0/12"),
						Router: netip.MustParseAddr("192.168.2.20").AsSlice(),
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := toDHCPData(tt.in)
			if tt.shouldErr && err == nil {
				t.Fatal("expected error")
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestToNetbootData(t *testing.T) {
	tests := map[string]struct {
		in        *tinkerbell.Netboot
		want      *Netboot
		shouldErr bool
	}{
		"nil input":    {in: nil, shouldErr: true},
		"bad ipxe url": {in: &tinkerbell.Netboot{IPXE: &tinkerbell.IPXE{URL: "bad"}}, shouldErr: true},
		"successful":   {in: &tinkerbell.Netboot{IPXE: &tinkerbell.IPXE{URL: "http://example.com/ipxe.ipxe"}}, want: &Netboot{IPXEScriptURL: &url.URL{Scheme: "http", Host: "example.com", Path: "/ipxe.ipxe"}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := toNetbootData(tt.in, "")
			if tt.shouldErr && err == nil {
				t.Fatal("expected error")
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
