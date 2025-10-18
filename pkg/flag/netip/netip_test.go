package netip

import (
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestAddrPortSet(t *testing.T) {
	tests := map[string]struct {
		input       string
		want        netip.AddrPort
		expectError bool
	}{
		"empty": {
			input:       "",
			expectError: false,
		},
		"valid ipv4 with port": {
			input:       "192.168.1.1:8080",
			want:        netip.MustParseAddrPort("192.168.1.1:8080"),
			expectError: false,
		},
		"valid ipv6 with port": {
			input:       "[2001:db8::1]:8080",
			want:        netip.MustParseAddrPort("[2001:db8::1]:8080"),
			expectError: false,
		},
		"invalid format": {
			input:       "192.168.1.1",
			expectError: true,
		},
		"invalid port": {
			input:       "192.168.1.1:99999",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ap := &AddrPort{AddrPort: new(netip.AddrPort)}
			err := ap.Set(tc.input)

			if tc.expectError && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}

			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectError && tc.input != "" && *ap.AddrPort != tc.want {
				t.Errorf("got %v, want %v", ap.AddrPort, tc.want)
			}
		})
	}
}

func TestAddrPortReset(t *testing.T) {
	ap := &AddrPort{AddrPort: new(netip.AddrPort)}
	*ap.AddrPort = netip.MustParseAddrPort("192.168.1.1:8080")

	if err := ap.Reset(); err != nil {
		t.Errorf("unexpected error from Reset: %v", err)
	}

	emptyAddrPort := netip.AddrPort{}
	if *ap.AddrPort != emptyAddrPort {
		t.Errorf("Reset didn't set to zero value; got %v", ap.AddrPort)
	}
}

func TestAddrPortType(t *testing.T) {
	ap := &AddrPort{AddrPort: new(netip.AddrPort)}
	if got := ap.Type(); got != "addr:port" {
		t.Errorf("Type() = %q, want %q", got, "addr:port")
	}
}

func TestAddrSet(t *testing.T) {
	tests := map[string]struct {
		input       string
		want        netip.Addr
		expectError bool
	}{
		"empty": {
			input:       "",
			expectError: false,
		},
		"valid ipv4": {
			input:       "192.168.1.1",
			want:        netip.MustParseAddr("192.168.1.1"),
			expectError: false,
		},
		"valid ipv6": {
			input:       "2001:db8::1",
			want:        netip.MustParseAddr("2001:db8::1"),
			expectError: false,
		},
		"invalid format": {
			input:       "not-an-ip",
			expectError: true,
		},
		"invalid ipv4": {
			input:       "192.168.1.300",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr := &Addr{Addr: new(netip.Addr)}
			err := addr.Set(tc.input)

			if tc.expectError && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}

			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectError && tc.input != "" && *addr.Addr != tc.want {
				t.Errorf("got %v, want %v", addr.Addr, tc.want)
			}
		})
	}
}

func TestAddrReset(t *testing.T) {
	addr := &Addr{Addr: new(netip.Addr)}
	*addr.Addr = netip.MustParseAddr("192.168.1.1")

	if err := addr.Reset(); err != nil {
		t.Errorf("unexpected error from Reset: %v", err)
	}

	emptyAddr := netip.Addr{}
	if *addr.Addr != emptyAddr {
		t.Errorf("Reset didn't set to zero value; got %v", addr.Addr)
	}
}

func TestAddrType(t *testing.T) {
	addr := &Addr{Addr: new(netip.Addr)}
	if got := addr.Type(); got != "addr" {
		t.Errorf("Type() = %q, want %q", got, "addr")
	}
}

func TestAddrString(t *testing.T) {
	tests := map[string]struct {
		addr *Addr
		want string
	}{
		"nil addr": {
			addr: nil,
			want: "",
		},
		"nil inner addr": {
			addr: &Addr{Addr: nil},
			want: "",
		},
		"valid addr": {
			addr: &Addr{Addr: func() *netip.Addr { a := netip.MustParseAddr("192.168.1.1"); return &a }()},
			want: "192.168.1.1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.addr.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrefixSet(t *testing.T) {
	tests := map[string]struct {
		input       string
		want        netip.Prefix
		expectError bool
	}{
		"empty": {
			input:       "",
			expectError: false,
		},
		"valid ipv4 prefix": {
			input:       "192.168.1.0/24",
			want:        netip.MustParsePrefix("192.168.1.0/24"),
			expectError: false,
		},
		"valid ipv6 prefix": {
			input:       "2001:db8::/32",
			want:        netip.MustParsePrefix("2001:db8::/32"),
			expectError: false,
		},
		"invalid format": {
			input:       "not-a-prefix",
			expectError: true,
		},
		"invalid prefix length": {
			input:       "192.168.1.0/33",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			prefix := &Prefix{Prefix: new(netip.Prefix)}
			err := prefix.Set(tc.input)

			if tc.expectError && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}

			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectError && tc.input != "" && *prefix.Prefix != tc.want {
				t.Errorf("got %v, want %v", prefix.Prefix, tc.want)
			}
		})
	}
}

func TestPrefixType(t *testing.T) {
	prefix := &Prefix{Prefix: new(netip.Prefix)}
	if got := prefix.Type(); got != "prefix" {
		t.Errorf("Type() = %q, want %q", got, "prefix")
	}
}

func TestPrefixListSet(t *testing.T) {
	tests := map[string]struct {
		input       string
		want        []netip.Prefix
		expectError bool
	}{
		"empty": {
			input:       "",
			want:        nil,
			expectError: false,
		},
		"single prefix": {
			input: "192.168.1.0/24",
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
			},
			expectError: false,
		},
		"multiple prefixes": {
			input: "192.168.1.0/24,10.0.0.0/8,2001:db8::/32",
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("2001:db8::/32"),
			},
			expectError: false,
		},
		"one invalid prefix": {
			input:       "192.168.1.0/24,invalid/33",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var prefixes []netip.Prefix
			pl := &PrefixList{PrefixList: &prefixes}
			err := pl.Set(tc.input)

			if tc.expectError && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}

			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectError {
				if diff := cmp.Diff(prefixes, tc.want, cmpopts.IgnoreUnexported(netip.Prefix{})); diff != "" {
					t.Errorf("unexpected difference: (-got +want):\n%s", diff)
				}
			}
		})
	}
}

func TestPrefixListReset(t *testing.T) {
	prefixes := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}

	pl := &PrefixList{PrefixList: &prefixes}

	if err := pl.Reset(); err != nil {
		t.Errorf("unexpected error from Reset: %v", err)
	}

	if len(prefixes) != 0 {
		t.Errorf("Reset didn't clear slice; got %v", prefixes)
	}
}

func TestPrefixListType(t *testing.T) {
	var prefixes []netip.Prefix
	pl := &PrefixList{PrefixList: &prefixes}

	if got := pl.Type(); got != "prefix list" {
		t.Errorf("Type() = %q, want %q", got, "prefix list")
	}
}

func TestToPrefixList(t *testing.T) {
	var prefixes []netip.Prefix
	pl := ToPrefixList(&prefixes)

	if pl.PrefixList != &prefixes {
		t.Errorf("ToPrefixList didn't set PrefixList field correctly")
	}
}

func TestPrefixListString(t *testing.T) {
	tests := map[string]struct {
		prefixes []netip.Prefix
		want     string
	}{
		"empty": {
			prefixes: []netip.Prefix{},
			want:     "",
		},
		"single prefix": {
			prefixes: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
			},
			want: "192.168.1.0/24",
		},
		"multiple prefixes": {
			prefixes: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("2001:db8::/32"),
			},
			want: "192.168.1.0/24,10.0.0.0/8,2001:db8::/32",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pl := &PrefixList{PrefixList: &tc.prefixes}

			if got := pl.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrefixListSlice(t *testing.T) {
	tests := map[string]struct {
		prefixes []netip.Prefix
		want     []string
	}{
		"empty": {
			prefixes: []netip.Prefix{},
			want:     nil,
		},
		"single prefix": {
			prefixes: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
			},
			want: []string{"192.168.1.0/24"},
		},
		"multiple prefixes": {
			prefixes: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("2001:db8::/32"),
			},
			want: []string{"192.168.1.0/24", "10.0.0.0/8", "2001:db8::/32"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pl := &PrefixList{PrefixList: &tc.prefixes}

			got := pl.Slice()
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("Slice() unexpected difference: (-got +want):\n%s", diff)
			}
		})
	}
}
