package dhcp

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func boolPtr(b bool) *bool { return &b }

func TestConvertByMac(t *testing.T) {
	tests := map[string]struct {
		mac       net.HardwareAddr
		hw        *tinkerbell.Hardware
		want      Hardware
		shouldErr bool
	}{
		"nil hardware": {
			mac:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw:        nil,
			shouldErr: true,
		},
		"empty interfaces": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw:  &tinkerbell.Hardware{},
			// no matching interface means transform gets a zero-value Interface which has nil DHCP/Netboot
			shouldErr: true,
		},
		"no matching mac": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "00:11:22:33:44:55",
								Hostname: "other",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.255.255.0",
									Gateway: "10.0.0.254",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			// No match means zero-value Interface -> nil DHCP -> transform error
			shouldErr: true,
		},
		"matching mac with full DHCP and netboot": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "test-host",
								IP: &tinkerbell.IP{
									Address: "192.168.1.10",
									Netmask: "255.255.255.0",
									Gateway: "192.168.1.1",
								},
							},
							Netboot: &tinkerbell.Netboot{
								AllowPXE: boolPtr(true),
								IPXE:     &tinkerbell.IPXE{URL: "http://example.com/auto.ipxe"},
							},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("192.168.1.10"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					DefaultGateway:   netip.MustParseAddr("192.168.1.1"),
					Hostname:         "test-host",
					BroadcastAddress: netip.MustParseAddr("192.168.1.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot:  true,
					IPXEScriptURL: &url.URL{Scheme: "http", Host: "example.com", Path: "/auto.ipxe"},
				},
			},
		},
		"matching mac among multiple interfaces": {
			mac: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "wrong",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.0.0.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(false)},
						},
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "00:11:22:33:44:55",
								Hostname: "correct",
								IP: &tinkerbell.IP{
									Address: "10.0.0.2",
									Netmask: "255.0.0.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
					IPAddress:        netip.MustParseAddr("10.0.0.2"),
					SubnetMask:       net.IPv4Mask(255, 0, 0, 0),
					Hostname:         "correct",
					BroadcastAddress: netip.MustParseAddr("10.255.255.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"with isoboot": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "iso-host",
								IP: &tinkerbell.IP{
									Address: "192.168.2.5",
									Netmask: "255.255.255.0",
									Gateway: "192.168.2.1",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
							Isoboot: &tinkerbell.Isoboot{SourceISO: "http://example.com/hook.iso"},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("192.168.2.5"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					DefaultGateway:   netip.MustParseAddr("192.168.2.1"),
					Hostname:         "iso-host",
					BroadcastAddress: netip.MustParseAddr("192.168.2.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
				Isoboot: &Isoboot{SourceISO: &url.URL{Scheme: "http", Host: "example.com", Path: "/hook.iso"}},
			},
		},
		"with metadata facility": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.255.255.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(false)},
						},
					},
					Metadata: &tinkerbell.HardwareMetadata{
						Facility: &tinkerbell.MetadataFacility{FacilityCode: "dc1"},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("10.0.0.1"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					BroadcastAddress: netip.MustParseAddr("10.0.0.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					Facility: "dc1",
				},
			},
		},
		"with agent id": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					AgentID: "custom-worker-1",
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "agent-host",
								IP: &tinkerbell.IP{
									Address: "10.0.0.5",
									Netmask: "255.255.255.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				AgentID: "custom-worker-1",
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("10.0.0.5"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					BroadcastAddress: netip.MustParseAddr("10.0.0.255"),
					Hostname:         "agent-host",
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"ipv6 address no netmask": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "ipv6-host",
								IP: &tinkerbell.IP{
									Address: "2001:db8::1",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:  netip.MustParseAddr("2001:db8::1"),
					Hostname:   "ipv6-host",
					LeaseTime:  0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"ipv6 address with netmask errors": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkerbell.IP{
									Address: "2001:db8::1",
									Netmask: "255.255.255.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			shouldErr: true,
		},
		"ipv6 gateway with ipv6 ip": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "ipv6-gw",
								IP: &tinkerbell.IP{
									Address: "2001:db8::1",
									Gateway: "2001:db8::fffe",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:     net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:      netip.MustParseAddr("2001:db8::1"),
					DefaultGateway: netip.MustParseAddr("2001:db8::fffe"),
					Hostname:       "ipv6-gw",
					LeaseTime:      0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"nameservers with invalid in middle preserved": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.255.255.0",
								},
								NameServers: []string{"8.8.8.8", "not-an-ip", "8.8.4.4"},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("10.0.0.1"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					BroadcastAddress: netip.MustParseAddr("10.0.0.255"),
					NameServers:      []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4")},
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"ipv4 netmask as ipv6 errors": {
			mac: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "2001:db8::1",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			shouldErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ConvertByMac(context.Background(), tt.mac, tt.hw)
			if tt.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatalf("mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestConvertByIP(t *testing.T) {
	tests := map[string]struct {
		ip        net.IP
		hw        *tinkerbell.Hardware
		want      Hardware
		shouldErr bool
	}{
		"nil hardware": {
			ip:        net.ParseIP("192.168.1.10"),
			hw:        nil,
			shouldErr: true,
		},
		"empty interfaces": {
			ip:        net.ParseIP("192.168.1.10"),
			hw:        &tinkerbell.Hardware{},
			shouldErr: true,
		},
		"no matching ip": {
			ip: net.ParseIP("192.168.1.10"),
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.255.255.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			// No matching IP -> zero Interface -> nil DHCP -> transform error
			shouldErr: true,
		},
		"matching ip": {
			ip: net.ParseIP("192.168.1.10"),
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "by-ip",
								IP: &tinkerbell.IP{
									Address: "192.168.1.10",
									Netmask: "255.255.255.0",
									Gateway: "192.168.1.1",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("192.168.1.10"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					DefaultGateway:   netip.MustParseAddr("192.168.1.1"),
					Hostname:         "by-ip",
					BroadcastAddress: netip.MustParseAddr("192.168.1.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"with agent id": {
			ip: net.ParseIP("192.168.1.10"),
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					AgentID: "custom-worker-2",
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "agent-host",
								IP: &tinkerbell.IP{
									Address: "192.168.1.10",
									Netmask: "255.255.255.0",
									Gateway: "192.168.1.1",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				AgentID: "custom-worker-2",
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
					IPAddress:        netip.MustParseAddr("192.168.1.10"),
					SubnetMask:       net.IPv4Mask(255, 255, 255, 0),
					DefaultGateway:   netip.MustParseAddr("192.168.1.1"),
					BroadcastAddress: netip.MustParseAddr("192.168.1.255"),
					Hostname:         "agent-host",
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
		"matching ip among multiple interfaces": {
			ip: net.ParseIP("10.0.0.2"),
			hw: &tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "aa:bb:cc:dd:ee:ff",
								Hostname: "wrong",
								IP: &tinkerbell.IP{
									Address: "10.0.0.1",
									Netmask: "255.0.0.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(false)},
						},
						{
							DHCP: &tinkerbell.DHCP{
								MAC:      "00:11:22:33:44:55",
								Hostname: "correct",
								IP: &tinkerbell.IP{
									Address: "10.0.0.2",
									Netmask: "255.0.0.0",
								},
							},
							Netboot: &tinkerbell.Netboot{AllowPXE: boolPtr(true)},
						},
					},
				},
			},
			want: Hardware{
				DHCP: &DHCP{
					MACAddress:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
					IPAddress:        netip.MustParseAddr("10.0.0.2"),
					SubnetMask:       net.IPv4Mask(255, 0, 0, 0),
					Hostname:         "correct",
					BroadcastAddress: netip.MustParseAddr("10.255.255.255"),
					LeaseTime:        0,
				},
				Netboot: &Netboot{
					AllowNetboot: true,
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ConvertByIP(context.Background(), tt.ip, tt.hw)
			if tt.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatalf("mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
