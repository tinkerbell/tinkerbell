package flag

import (
	"strings"
	"testing"
)

// singleFamilyFlags have no counterpart in the other address family, by
// protocol design rather than oversight. Each entry documents why.
func singleFamilyFlags() map[string]string {
	return map[string]string{
		// DHCPv4 option 54 server identifier. DHCPv6 identifies by DUID.
		"dhcp-ip-for-packet-v4": "DHCPv6 identifies the server by DUID, not address",
		// DHCPv6 identifies the server by DUID; DHCPv4 has no equivalent.
		"dhcp-server-duid-v6": "DHCPv4 has no DUID",
		// Derived addressing is DHCPv6 only.
		"dhcp-derived-direct-address-pool-v6":  "derived addressing is DHCPv6 only",
		"dhcp-derived-relay-address-prefix-v6": "derived addressing is DHCPv6 only",
		// Static IPAM in patched ISOs is IPv4 only; IPv6 uses SLAAC or DHCPv6.
		"iso-static-ipam-enabled-v4": "static IPAM in ISOs is IPv4 only",
	}
}

// TestFamilyFlagsArePaired keeps the flag surface honest: a family-scoped flag
// must exist for both families unless it is listed, with a reason, above.
func TestFamilyFlagsArePaired(t *testing.T) {
	registered := registeredFlagNames(t)
	exempt := singleFamilyFlags()

	for name := range registered {
		base, twin, ok := familyTwin(name)
		if !ok {
			continue
		}
		if _, allowed := exempt[name]; allowed {
			continue
		}
		if !registered[twin] {
			t.Errorf("%q has no counterpart %q; add it, or list %q in singleFamilyFlags with a reason", name, twin, base)
		}
	}

	for name := range exempt {
		if !registered[name] {
			t.Errorf("singleFamilyFlags lists %q, which is not registered", name)
		}
	}
}

// familyTwin returns the base name and the other family's flag name.
func familyTwin(name string) (base, twin string, ok bool) {
	if base, found := strings.CutSuffix(name, "-v4"); found {
		return base, base + "-v6", true
	}
	if base, found := strings.CutSuffix(name, "-v6"); found {
		return base, base + "-v4", true
	}
	return "", "", false
}
