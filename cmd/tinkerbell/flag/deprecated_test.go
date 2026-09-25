package flag

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/smee"
)

func TestRenameDeprecatedArgs(t *testing.T) {
	tests := map[string]struct {
		args     []string
		want     []string
		warnings int
	}{
		"separate value": {
			args:     []string{"--dhcpv6-mode", "stateless"},
			want:     []string{"--dhcp-mode-v6", "stateless"},
			warnings: 1,
		},
		"inline value": {
			args:     []string{"--dhcpv6-mode=stateless"},
			want:     []string{"--dhcp-mode-v6=stateless"},
			warnings: 1,
		},
		"inline value containing equals is preserved": {
			args: []string{"--ipxe-http-script-extra-kernel-args=a=b"},
			want: []string{
				"--ipxe-http-script-extra-kernel-args-v4=a=b",
				"--ipxe-http-script-extra-kernel-args-v6=a=b",
			},
			warnings: 1,
		},
		// The retired flag reached machines of both families, so it still must.
		"separate value fans out to every replacement": {
			args: []string{"--ipxe-http-script-extra-kernel-args", "a=b"},
			want: []string{
				"--ipxe-http-script-extra-kernel-args-v4=a=b",
				"--ipxe-http-script-extra-kernel-args-v6=a=b",
			},
			warnings: 1,
		},
		"a fanned out flag keeps surrounding args": {
			args: []string{"--log-level=1", "--ipxe-http-script-extra-kernel-args", "a=b", "--dhcp-mode-v4=proxy"},
			want: []string{
				"--log-level=1",
				"--ipxe-http-script-extra-kernel-args-v4=a=b",
				"--ipxe-http-script-extra-kernel-args-v6=a=b",
				"--dhcp-mode-v4=proxy",
			},
			warnings: 1,
		},
		"current names are untouched": {
			args: []string{"--dhcp-mode-v6", "stateless"},
			want: []string{"--dhcp-mode-v6", "stateless"},
		},
		"unknown flags are untouched": {
			args: []string{"--not-a-flag", "x"},
			want: []string{"--not-a-flag", "x"},
		},
		"a value that looks like a deprecated flag is untouched": {
			args: []string{"--backend-file-path", "public-ipv6"},
			want: []string{"--backend-file-path", "public-ipv6"},
		},
		"empty args": {
			args: []string{},
			want: []string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, warnings := RenameDeprecatedArgs(tt.args)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
			if len(warnings) != tt.warnings {
				t.Errorf("warnings = %d, want %d: %v", len(warnings), tt.warnings, warnings)
			}
		})
	}
}

func TestRenameDeprecatedEnv(t *testing.T) {
	t.Run("copies onto the current name", func(t *testing.T) {
		t.Setenv("TINKERBELL_DHCPV6_MODE", "stateless")

		warnings, err := RenameDeprecatedEnv()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(warnings), 1; got != want {
			t.Fatalf("warnings = %d, want %d: %v", got, want, warnings)
		}
		if got, want := os.Getenv("TINKERBELL_DHCP_MODE_V6"), "stateless"; got != want {
			t.Errorf("TINKERBELL_DHCP_MODE_V6 = %q, want %q", got, want)
		}
	})

	t.Run("an explicitly set current name wins", func(t *testing.T) {
		t.Setenv("TINKERBELL_DHCPV6_MODE", "stateless")
		t.Setenv("TINKERBELL_DHCP_MODE_V6", "reservation")

		warnings, err := RenameDeprecatedEnv()
		if err != nil {
			t.Fatal(err)
		}
		if len(warnings) != 0 {
			t.Errorf("warnings = %v, want none", warnings)
		}
		if got, want := os.Getenv("TINKERBELL_DHCP_MODE_V6"), "reservation"; got != want {
			t.Errorf("TINKERBELL_DHCP_MODE_V6 = %q, want %q", got, want)
		}
	})

	// ff distinguishes set-but-empty from unset, so an empty value must carry over.
	t.Run("an empty value is copied", func(t *testing.T) {
		t.Setenv("TINKERBELL_DHCPV6_MODE", "")

		if _, err := RenameDeprecatedEnv(); err != nil {
			t.Fatal(err)
		}
		if _, ok := os.LookupEnv("TINKERBELL_DHCP_MODE_V6"); !ok {
			t.Error("TINKERBELL_DHCP_MODE_V6 not set")
		}
	})
}

// TestDeprecatedNamesResolve guards against a typo in deprecatedNames silently
// retiring a flag to a name that does not exist.
func TestDeprecatedNamesResolve(t *testing.T) {
	registered := registeredFlagNames(t)

	for from, to := range deprecatedNames() {
		if registered[from] {
			t.Errorf("%q is listed as deprecated but is still registered", from)
		}
		for _, n := range to {
			if !registered[n] {
				t.Errorf("%q is deprecated in favour of %q, which is not registered", from, n)
			}
		}
	}
}

// TestDeprecatedArgsReachTheirFlag parses every deprecated name through the shim
// to prove the rewrite lands on a real flag.
func TestDeprecatedArgsReachTheirFlag(t *testing.T) {
	for from := range deprecatedNames() {
		t.Run(from, func(t *testing.T) {
			fs := newFullFlagSet(t)
			args, warnings := RenameDeprecatedArgs([]string{"--" + from + "=" + probeValue(from)})
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want 1", warnings)
			}
			if err := ff.NewFlagSet("probe").SetParent(fs).Parse(args); err != nil {
				t.Fatalf("parse %v: %v", args, err)
			}
		})
	}
}

// probeValue returns a value each deprecated flag will accept. Most flags take
// any string; the ones that parse their input need a well-formed one.
func probeValue(name string) string {
	switch name {
	case "public-ipv4", "bind-address", "dhcp-bind-addr", "dhcp-ip-for-packet", "dhcp-syslog-ip", "dhcp-tftp-ip":
		return "192.0.2.1"
	case "public-ipv6", "dhcpv6-bind-addr", "dhcpv6-syslog-ip", "dhcpv6-tftp-ip":
		return "2001:db8::1"
	case "dhcpv6-derived-direct-address-pool":
		return "2001:db8::/64"
	case "dhcpv6-default-name-servers":
		return "2001:db8::53"
	case "dhcpv6-default-domain-search-list":
		return "example.com"
	case "dhcp-mode":
		return string(smee.DHCPModeReservation)
	case "dhcpv6-mode":
		return string(smee.DHCPv6ModeStateless)
	case "http-port", "https-port", "dhcp-tftp-port", "dhcpv6-tftp-port", "dhcpv6-bind-port",
		"dhcp-ipxe-http-binary-port", "dhcpv6-ipxe-http-binary-port", "dhcp-ipxe-http-script-port",
		"dhcpv6-ipxe-http-script-port", "dhcpv6-derived-relay-address-prefix", "secondstar-port",
		"syslog-bind-port", "tftp-server-bind-port", "tink-server-bind-port":
		return "1"
	case "dhcp-enabled", "dhcpv6-enabled", "dhcp-enable-netboot-options",
		"dhcpv6-enable-netboot-options", "dhcp-ipxe-http-script-prepend-mac",
		"dhcpv6-ipxe-http-script-prepend-mac", "iso-static-ipam-enabled":
		return "true"
	case "tink-server-bind-addr", "syslog-bind-addr", "tftp-server-bind-addr":
		return "0.0.0.0"
	case "ipxe-http-script-osie-url":
		return "http://192.0.2.1/hook"
	default:
		return "probe"
	}
}
