package dhcpv6

import (
	"net"
	"net/netip"
	"net/url"
	"testing"

	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
)

func TestNewNetbootRejectsUnusableConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NetbootConfig)
	}{
		{
			name: "missing TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.AddrPort{}
			},
		},
		{
			name: "unspecified TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::]:69")
			},
		},
		{
			name: "IPv4-mapped unspecified TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::ffff:0.0.0.0]:69")
			},
		},
		{
			name: "IPv4 unicast TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("192.0.2.1:69")
			},
		},
		{
			name: "IPv4-mapped unicast TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::ffff:192.0.2.1]:69")
			},
		},
		{
			name: "IPv4 loopback TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("127.0.0.1:69")
			},
		},
		{
			name: "IPv6 loopback TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::1]:69")
			},
		},
		{
			name: "IPv4-mapped loopback TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::ffff:127.0.0.1]:69")
			},
		},
		{
			name: "broadcast TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("255.255.255.255:69")
			},
		},
		{
			name: "IPv4-mapped broadcast TFTP address",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[::ffff:255.255.255.255]:69")
			},
		},
		{
			name: "zero TFTP port",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerTFTP = netip.MustParseAddrPort("[2001:db8::1]:0")
			},
		},
		{
			name: "missing HTTP binary URL",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = nil
			},
		},
		{
			name: "HTTP binary URL without host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http:///ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with unspecified host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://[::]/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with IPv4-mapped unspecified host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://[::ffff:0.0.0.0]/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with IPv4 unicast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://192.0.2.1/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with IPv4-mapped unicast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://[::ffff:192.0.2.1]/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with loopback host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://127.0.0.1/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with broadcast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://255.255.255.255/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with unsupported scheme",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "tftp://boot.example/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with zero port",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://boot.example:0/ipxe/binary")
			},
		},
		{
			name: "HTTP binary URL with out-of-range port",
			mutate: func(config *NetbootConfig) {
				config.IPXEBinServerHTTP = mustParseURL(t, "http://boot.example:65536/ipxe/binary")
			},
		},
		{
			name: "missing HTTP script URL",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = nil
			},
		},
		{
			name: "HTTP script URL without host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http:///ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with unspecified host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://[::]/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with loopback host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://[::1]/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with IPv4-mapped broadcast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://[::ffff:255.255.255.255]/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with IPv4 unicast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://192.0.2.1/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with IPv4-mapped unicast host",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://[::ffff:192.0.2.1]/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with unsupported scheme",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "tftp://boot.example/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with zero port",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://boot.example:0/ipxe/script/auto6.ipxe")
			},
		},
		{
			name: "HTTP script URL with out-of-range port",
			mutate: func(config *NetbootConfig) {
				config.IPXEScriptURL = mustParseURL(t, "http://boot.example:65536/ipxe/script/auto6.ipxe")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validNetbootConfig(t)
			test.mutate(&config)
			if _, err := NewNetboot(config); err == nil {
				t.Fatal("expected invalid DHCPv6 netboot configuration to fail")
			}
		})
	}
}

func TestNewNetbootAcceptsIPv6HTTPHosts(t *testing.T) {
	config := validNetbootConfig(t)
	config.IPXEBinServerHTTP = mustParseURL(t, "http://[2001:db8::1]/ipxe/binary")
	config.IPXEScriptURL = mustParseURL(t, "https://[2001:db8::1]/ipxe/script/auto6.ipxe")

	if _, err := NewNetboot(config); err != nil {
		t.Fatalf("expected IPv6 HTTP hosts to be accepted: %v", err)
	}
}

func TestNewNetbootAcceptsHTTPPortBoundaries(t *testing.T) {
	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			config := validNetbootConfig(t)
			config.IPXEBinServerHTTP = mustParseURL(t, "http://boot.example:"+port+"/ipxe/binary")
			config.IPXEScriptURL = mustParseURL(t, "http://boot.example:"+port+"/ipxe/script/auto6.ipxe")

			if _, err := NewNetboot(config); err != nil {
				t.Fatalf("expected valid HTTP port to be accepted: %v", err)
			}
		})
	}
}

func TestNewNetbootAcceptsScriptURLWithoutFilename(t *testing.T) {
	for name, scriptURL := range map[string]string{
		"empty path":          "https://boot.example",
		"root path":           "https://boot.example/",
		"trailing slash path": "https://boot.example/ipxe/script/",
	} {
		t.Run(name, func(t *testing.T) {
			config := validNetbootConfig(t)
			config.IPXEScriptURL = mustParseURL(t, scriptURL)
			config.InjectMacAddress = false

			netboot, err := NewNetboot(config)
			if err != nil {
				t.Fatalf("expected script URL to be accepted: %v", err)
			}
			got, err := netboot.BootURL(Info{UserClasses: []dhcp.UserClass{dhcp.IPXE}}, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if got != scriptURL {
				t.Fatalf("unexpected script URL: got %q want %q", got, scriptURL)
			}
		})
	}
}

func TestDisabledNetbootOmitsBootURL(t *testing.T) {
	disabled := DisabledNetboot{}
	if options := disabled.InfoOptions(); len(options) != 0 {
		t.Fatalf("expected no disabled netboot info options, got %d", len(options))
	}
	bootURL, err := disabled.BootURL(Info{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if bootURL != "" {
		t.Fatalf("expected no disabled netboot URL, got %q", bootURL)
	}
}

func TestNewNetbootCopiesURLs(t *testing.T) {
	config := validNetbootConfig(t)
	configured, err := NewNetboot(config)
	if err != nil {
		t.Fatal(err)
	}

	config.IPXEBinServerHTTP.Host = "changed.example"
	config.IPXEScriptURL.Host = "changed.example"

	if got := configured.ipxeBinServerHTTP.Host; got != "boot.example" {
		t.Fatalf("HTTP binary URL was mutated after construction: %s", got)
	}
	if got := configured.ipxeScriptURL.Host; got != "boot.example" {
		t.Fatalf("HTTP script URL was mutated after construction: %s", got)
	}
}

func TestNewNetbootNormalizesOpaqueHTTPURLs(t *testing.T) {
	config := validNetbootConfig(t)
	config.IPXEBinServerHTTP = &url.URL{Scheme: "http", Opaque: "//boot.example/ipxe/binary"}
	config.IPXEScriptURL = &url.URL{Scheme: "http", Opaque: "//boot.example/ipxe/script/auto6.ipxe"}
	configured, err := NewNetboot(config)
	if err != nil {
		t.Fatal(err)
	}

	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	binaryURL, err := configured.BootURL(Info{
		Arch:       iana.EFI_X86_64_HTTP,
		IPXEBinary: constant.IPXEBinaryIPXEEFI.String(),
		Mac:        mac,
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://boot.example/ipxe/binary/00:01:02:03:04:05/ipxe.efi"; binaryURL != want {
		t.Fatalf("unexpected binary URL: got %q want %q", binaryURL, want)
	}

	scriptURL, err := configured.BootURL(Info{
		Mac:         mac,
		UserClasses: []dhcp.UserClass{dhcp.IPXE},
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://boot.example/ipxe/script/00:01:02:03:04:05/auto6.ipxe"; scriptURL != want {
		t.Fatalf("unexpected script URL: got %q want %q", scriptURL, want)
	}
}

func TestBootURLMACFormats(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	tests := map[string]struct {
		mac    net.HardwareAddr
		format constant.MACFormat
		path   string
	}{
		"default":        {mac: mac, path: "/00:01:02:03:04:05/ipxe.efi"},
		"colon":          {mac: mac, format: constant.MacAddrFormatColon, path: "/00:01:02:03:04:05/ipxe.efi"},
		"dot":            {mac: mac, format: constant.MacAddrFormatDot, path: "/0001.0203.0405/ipxe.efi"},
		"dash":           {mac: mac, format: constant.MacAddrFormatDash, path: "/00-01-02-03-04-05/ipxe.efi"},
		"no delimiter":   {mac: mac, format: constant.MacAddrFormatNoDelimiter, path: "/000102030405/ipxe.efi"},
		"empty format":   {mac: mac, format: constant.MacAddrFormatEmpty, path: "/ipxe.efi"},
		"unknown format": {mac: mac, format: constant.MACFormat("unknown"), path: "/00:01:02:03:04:05/ipxe.efi"},
		"nil MAC":        {path: "/ipxe.efi"},
		"empty MAC":      {mac: net.HardwareAddr{}, format: constant.MacAddrFormatDot, path: "/ipxe.efi"},
		"dot EUI-64":     {mac: net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, format: constant.MacAddrFormatDot, path: "/0001.0203.0405.0607/ipxe.efi"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config := validNetbootConfig(t)
			config.InjectMacAddrFormat = tt.format
			netboot, err := NewNetboot(config)
			if err != nil {
				t.Fatal(err)
			}
			for _, protocol := range []struct {
				name string
				arch iana.Arch
				base string
			}{
				{name: "HTTP", arch: iana.EFI_X86_64_HTTP, base: "http://boot.example/ipxe/binary"},
				{name: "TFTP", arch: iana.EFI_X86_64, base: "tftp://[2001:db8::1]"},
			} {
				t.Run(protocol.name, func(t *testing.T) {
					info := Info{Arch: protocol.arch, Mac: tt.mac, IPXEBinary: "ipxe.efi"}
					for _, option := range netboot.InfoOptions() {
						option(&info)
					}
					got, err := netboot.BootURL(info, nil, "")
					if err != nil {
						t.Fatal(err)
					}
					if want := protocol.base + tt.path; got != want {
						t.Fatalf("unexpected boot URL: got %q want %q", got, want)
					}
				})
			}
		})
	}
}

func TestNetbootArchMappingIsCopiedAtBoundaries(t *testing.T) {
	const arch = iana.EFI_X86_64

	config := validNetbootConfig(t)
	config.IPXEArchMapping = map[iana.Arch]constant.IPXEBinary{
		arch: constant.IPXEBinaryIPXEEFI,
	}
	configured, err := NewNetboot(config)
	if err != nil {
		t.Fatal(err)
	}

	config.IPXEArchMapping[arch] = constant.IPXEBinarySNPAMD64
	first := Info{}
	for _, option := range configured.InfoOptions() {
		option(&first)
	}
	if got := first.ArchMappingOverride[arch]; got != constant.IPXEBinaryIPXEEFI {
		t.Fatalf("architecture mapping was mutated through constructor input: %q", got)
	}

	first.ArchMappingOverride[arch] = constant.IPXEBinarySNPAMD64
	second := Info{}
	for _, option := range configured.InfoOptions() {
		option(&second)
	}
	if got := second.ArchMappingOverride[arch]; got != constant.IPXEBinaryIPXEEFI {
		t.Fatalf("architecture mapping was mutated through Info options: %q", got)
	}
}

func validNetbootConfig(t *testing.T) NetbootConfig {
	t.Helper()
	return NetbootConfig{
		IPXEBinServerTFTP: netip.MustParseAddrPort("[2001:db8::1]:69"),
		IPXEBinServerHTTP: mustParseURL(t, "http://boot.example/ipxe/binary"),
		IPXEScriptURL:     mustParseURL(t, "http://boot.example/ipxe/script/auto6.ipxe"),
		InjectMacAddress:  true,
	}
}
