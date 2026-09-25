package flag

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// deprecatedNames maps retired flag names to their current equivalents. Address
// family is now always a trailing -v4 or -v6, so every name that carried an
// implicit family, or spelled it as a "dhcpv6-" infix, moved. A name maps to
// more than one replacement when it used to apply to both families.
func deprecatedNames() map[string][]string {
	return map[string][]string{
		// Globals.
		"public-ipv4":  {"public-ip-v4"},
		"public-ipv6":  {"public-ip-v6"},
		"bind-address": {"bind-address-v4"},
		"http-port":    {"http-port-v4"},
		"https-port":   {"https-port-v4"},

		// DHCP, previously unsuffixed for IPv4.
		"dhcp-enabled":                      {"dhcp-enabled-v4"},
		"dhcp-mode":                         {"dhcp-mode-v4"},
		"dhcp-enable-netboot-options":       {"dhcp-enable-netboot-options-v4"},
		"dhcp-bind-addr":                    {"dhcp-bind-addr-v4"},
		"dhcp-bind-interface":               {"dhcp-bind-interface-v4"},
		"dhcp-ip-for-packet":                {"dhcp-ip-for-packet-v4"},
		"dhcp-syslog-ip":                    {"dhcp-syslog-ip-v4"},
		"dhcp-tftp-ip":                      {"dhcp-tftp-ip-v4"},
		"dhcp-tftp-port":                    {"dhcp-tftp-port-v4"},
		"dhcp-ipxe-http-binary-scheme":      {"dhcp-ipxe-http-binary-scheme-v4"},
		"dhcp-ipxe-http-binary-host":        {"dhcp-ipxe-http-binary-host-v4"},
		"dhcp-ipxe-http-binary-port":        {"dhcp-ipxe-http-binary-port-v4"},
		"dhcp-ipxe-http-binary-path":        {"dhcp-ipxe-http-binary-path-v4"},
		"dhcp-ipxe-http-script-scheme":      {"dhcp-ipxe-http-script-scheme-v4"},
		"dhcp-ipxe-http-script-host":        {"dhcp-ipxe-http-script-host-v4"},
		"dhcp-ipxe-http-script-port":        {"dhcp-ipxe-http-script-port-v4"},
		"dhcp-ipxe-http-script-path":        {"dhcp-ipxe-http-script-path-v4"},
		"dhcp-ipxe-http-script-prepend-mac": {"dhcp-ipxe-http-script-prepend-mac-v4"},

		// DHCPv6, previously a "dhcpv6-" infix.
		"dhcpv6-enabled":                      {"dhcp-enabled-v6"},
		"dhcpv6-mode":                         {"dhcp-mode-v6"},
		"dhcpv6-enable-netboot-options":       {"dhcp-enable-netboot-options-v6"},
		"dhcpv6-bind-addr":                    {"dhcp-bind-addr-v6"},
		"dhcpv6-bind-port":                    {"dhcp-bind-port-v6"},
		"dhcpv6-bind-interface":               {"dhcp-bind-interface-v6"},
		"dhcpv6-server-duid":                  {"dhcp-server-duid-v6"},
		"dhcpv6-default-name-servers":         {"dhcp-default-name-servers-v6"},
		"dhcpv6-default-domain-search-list":   {"dhcp-default-domain-search-list-v6"},
		"dhcpv6-derived-direct-address-pool":  {"dhcp-derived-direct-address-pool-v6"},
		"dhcpv6-derived-relay-address-prefix": {"dhcp-derived-relay-address-prefix-v6"},
		"dhcpv6-syslog-ip":                    {"dhcp-syslog-ip-v6"},
		"dhcpv6-tftp-ip":                      {"dhcp-tftp-ip-v6"},
		"dhcpv6-tftp-port":                    {"dhcp-tftp-port-v6"},
		"dhcpv6-ipxe-http-binary-scheme":      {"dhcp-ipxe-http-binary-scheme-v6"},
		"dhcpv6-ipxe-http-binary-host":        {"dhcp-ipxe-http-binary-host-v6"},
		"dhcpv6-ipxe-http-binary-port":        {"dhcp-ipxe-http-binary-port-v6"},
		"dhcpv6-ipxe-http-binary-path":        {"dhcp-ipxe-http-binary-path-v6"},
		"dhcpv6-ipxe-http-script-scheme":      {"dhcp-ipxe-http-script-scheme-v6"},
		"dhcpv6-ipxe-http-script-host":        {"dhcp-ipxe-http-script-host-v6"},
		"dhcpv6-ipxe-http-script-port":        {"dhcp-ipxe-http-script-port-v6"},
		"dhcpv6-ipxe-http-script-path":        {"dhcp-ipxe-http-script-path-v6"},
		"dhcpv6-ipxe-http-script-prepend-mac": {"dhcp-ipxe-http-script-prepend-mac-v6"},

		// iPXE and ISO. The extra kernel args reached machines of both families.
		"ipxe-http-script-extra-kernel-args": {"ipxe-http-script-extra-kernel-args-v4", "ipxe-http-script-extra-kernel-args-v6"},
		"ipxe-http-script-osie-url":          {"ipxe-http-script-osie-url-v4"},
		"ipxe-script-syslog-fqdn":            {"ipxe-script-syslog-fqdn-v4"},
		"ipxe-script-tink-server-addr-port":  {"ipxe-script-tink-server-addr-port-v4"},
		"iso-static-ipam-enabled":            {"iso-static-ipam-enabled-v4"},

		// Syslog and TFTP.
		"syslog-bind-addr":      {"syslog-bind-addr-v4"},
		"syslog-bind-port":      {"syslog-bind-port-v4"},
		"tftp-server-bind-addr": {"tftp-server-bind-addr-v4"},
		"tftp-server-bind-port": {"tftp-server-bind-port-v4"},

		// Tink server and SecondStar.
		"tink-server-bind-addr": {"tink-server-bind-addr-v4"},
		"tink-server-bind-port": {"tink-server-bind-port-v4"},
		"secondstar-port":       {"secondstar-port-v4"},
	}
}

// RenameDeprecatedArgs rewrites retired flag names in args to their current
// names, returning the rewritten args and one warning per rename applied.
func RenameDeprecatedArgs(args []string) ([]string, []string) {
	current := deprecatedNames()
	out := make([]string, 0, len(args))
	var warnings []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			out = append(out, arg)
			continue
		}
		// ff only treats "--" as a long name prefix, so "-name" needs no handling.
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		to, ok := current[name]
		if !ok {
			out = append(out, arg)
			continue
		}
		// A replacement per family means repeating the flag, so a value given as
		// the next argument has to be pulled in and inlined.
		if len(to) > 1 && !hasValue && i+1 < len(args) {
			value, hasValue = args[i+1], true
			i++
		}

		for _, n := range to {
			if hasValue {
				out = append(out, "--"+n+"="+value)
				continue
			}
			out = append(out, "--"+n)
		}
		warnings = append(warnings, deprecationWarning("--"+name, flagNames(to)))
	}

	return out, warnings
}

// RenameDeprecatedEnv copies values from retired environment variables onto
// their current names, leaving an explicitly set current name alone. It returns
// one warning per value copied.
func RenameDeprecatedEnv() ([]string, error) {
	var warnings []string

	for from, to := range deprecatedNames() {
		fromKey := envKey(from)
		// ff distinguishes set-but-empty from unset, so copy the value verbatim.
		value, ok := os.LookupEnv(fromKey)
		if !ok {
			continue
		}
		for _, n := range to {
			toKey := envKey(n)
			if _, ok := os.LookupEnv(toKey); ok {
				continue
			}
			if err := os.Setenv(toKey, value); err != nil {
				return nil, fmt.Errorf("set %s from deprecated %s: %w", toKey, fromKey, err)
			}
			warnings = append(warnings, deprecationWarning(fromKey, toKey))
		}
	}
	slices.Sort(warnings) // map iteration order is random; keep warnings stable

	return warnings, nil
}

// flagNames renders names as a human readable list of long flags.
func flagNames(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "--" + n
	}
	return strings.Join(out, " and ")
}

// envKey returns the environment variable ff derives from a flag name.
func envKey(name string) string {
	return EnvVarPrefix + "_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func deprecationWarning(from, to string) string {
	return fmt.Sprintf("%s is deprecated and will be removed in a future release; use %s instead", from, to)
}
