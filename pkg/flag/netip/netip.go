// Package netip provides wrappers around net/netip types to implement the flag.Value interface.
// This allows these types to be used with the Go flag package for command-line flag parsing.
package netip

import (
	"fmt"
	"net/netip"
	"strings"
)

// AddrPort wraps a netip.AddrPort to implement the flag.Value interface.
// It represents an IP address and port number.
type AddrPort struct{ *netip.AddrPort }

// Set implements the flag.Value interface.
// Parses a string in the format "ip:port" and sets the AddrPort value.
// Returns an error if the string cannot be parsed or if the AddrPort is nil.
// An empty input string is treated as a no-op and returns nil.
func (a *AddrPort) Set(s string) error {
	if a == nil || a.AddrPort == nil {
		return fmt.Errorf("AddrPort is nil")
	}

	if s == "" {
		return nil
	}

	ip, err := netip.ParseAddrPort(strings.TrimSpace(s))
	if !ip.IsValid() || err != nil {
		return fmt.Errorf("failed to parse Addr:Port: %q: err: %v", s, err)
	}
	*a.AddrPort = ip

	return nil
}

// Reset sets the AddrPort to its zero value.
// Returns an error if the AddrPort is nil.
func (a *AddrPort) Reset() error {
	if a == nil || a.AddrPort == nil {
		return fmt.Errorf("AddrPort is nil")
	}

	*a.AddrPort = netip.AddrPort{}

	return nil
}

// Type implements the flag.Value interface.
// Returns the type of the flag as a string.
func (a *AddrPort) Type() string {
	return "addr:port"
}

// String returns the string representation of the AddrPort.
// Returns an empty string if the AddrPort is nil or invalid.
func (a *AddrPort) String() string {
	if a == nil || a.AddrPort == nil || !a.IsValid() {
		return ""
	}

	return a.AddrPort.String()
}

// Addr wraps a netip.Addr to implement the flag.Value interface.
// It represents an IP address without a port.
type Addr struct{ *netip.Addr }

// Set implements the flag.Value interface.
// Parses a string as an IP address and sets the Addr value.
// Returns an error if the string cannot be parsed or if the Addr is nil.
// An empty input string is treated as a no-op and returns nil.
func (a *Addr) Set(s string) error {
	if a == nil || a.Addr == nil {
		return fmt.Errorf("Addr is nil")
	}

	if s == "" {
		return nil
	}
	ip, err := netip.ParseAddr(s)
	if !ip.IsValid() || err != nil {
		return fmt.Errorf("failed to parse Address: %q", s)
	}
	*a.Addr = ip

	return nil
}

// Reset sets the Addr to its zero value.
// Returns an error if the Addr is nil.
func (a *Addr) Reset() error {
	if a == nil || a.Addr == nil {
		return fmt.Errorf("Addr is nil")
	}

	*a.Addr = netip.Addr{}

	return nil
}

// Type implements the flag.Value interface.
// Returns the type of the flag as a string.
func (a *Addr) Type() string {
	return "addr"
}

// String returns the string representation of the Addr.
// Returns an empty string if the Addr is nil or invalid.
func (a *Addr) String() string {
	if a == nil || a.Addr == nil || !a.IsValid() {
		return ""
	}

	return a.Addr.String()
}

// Prefix wraps a netip.Prefix to implement the flag.Value interface.
// It represents an IP network with address and mask (CIDR notation).
type Prefix struct{ *netip.Prefix }

// Set implements the flag.Value interface.
// Parses a string in CIDR notation (e.g., "192.168.0.0/24") and sets the Prefix value.
// Returns an error if the string cannot be parsed or if the Prefix is nil.
// An empty input string is treated as a no-op and returns nil.
func (p *Prefix) Set(s string) error {
	if p == nil || p.Prefix == nil {
		return fmt.Errorf("Prefix is nil")
	}

	if s == "" {
		return nil
	}
	ip, err := netip.ParsePrefix(s)
	if !ip.IsValid() || err != nil {
		return fmt.Errorf("failed to parse Prefix: %q", s)
	}
	*p.Prefix = ip

	return nil
}

// Type implements the flag.Value interface.
// Returns the type of the flag as a string.
func (p *Prefix) Type() string {
	return "prefix"
}

// Reset sets the Prefix to its zero value.
func (p *Prefix) Reset() error {
	if p == nil || p.Prefix == nil {
		return fmt.Errorf("Prefix is nil")
	}

	*p.Prefix = netip.Prefix{}

	return nil
}

// String returns the string representation of the Prefix.
// Returns an empty string if the Prefix is nil or invalid.
func (p *Prefix) String() string {
	if p == nil || p.Prefix == nil || !p.IsValid() {
		return ""
	}

	return p.Prefix.String()
}

// PrefixList wraps a slice of netip.Prefix to implement the flag.Value interface.
// It represents a list of IP networks specified in CIDR notation.
type PrefixList struct {
	PrefixList *[]netip.Prefix
}

// Set implements the flag.Value interface.
// Parses a comma-separated string of CIDR notations and sets the PrefixList value.
// Returns an error if any prefix cannot be parsed or if the PrefixList is nil.
// An empty input string is treated as a no-op and returns nil.
func (p *PrefixList) Set(s string) error {
	if p == nil || p.PrefixList == nil {
		return fmt.Errorf("PrefixList is nil")
	}

	if s == "" {
		return nil
	}
	pl := strings.Split(s, ",")
	results := make([]netip.Prefix, 0, len(pl))
	for _, prefix := range pl {
		ip, err := netip.ParsePrefix(prefix)
		if !ip.IsValid() || err != nil {
			return fmt.Errorf("failed to parse Prefix: %q", prefix)
		}
		results = append(results, ip)
	}
	*p.PrefixList = results

	return nil
}

// Reset sets the PrefixList to nil.
// Returns an error if the PrefixList is nil.
func (p *PrefixList) Reset() error {
	if p == nil || p.PrefixList == nil {
		return fmt.Errorf("PrefixList is nil")
	}

	*p.PrefixList = nil

	return nil
}

// Type implements the flag.Value interface.
// Returns the type of the flag as a string.
func (p *PrefixList) Type() string {
	return "prefix list"
}

// ToPrefixList creates a new PrefixList wrapper around a slice of netip.Prefix.
// This is a helper function to easily create a PrefixList from an existing slice.
func ToPrefixList(p *[]netip.Prefix) *PrefixList {
	pl := PrefixList{PrefixList: p}

	return &pl
}

// prefixesToStrings converts the prefix list to a slice of strings.
// Returns nil if the PrefixList is nil.
func (p *PrefixList) prefixesToStrings() []string {
	if p == nil || p.PrefixList == nil {
		return nil
	}

	var result []string
	for _, prefix := range *p.PrefixList {
		result = append(result, prefix.String())
	}

	return result
}

// String returns a comma-separated string of all prefixes.
// Returns an empty string if the PrefixList is nil.
func (p *PrefixList) String() string {
	strs := p.prefixesToStrings()
	if strs == nil {
		return ""
	}

	return strings.Join(strs, ",")
}

// Slice returns a slice of strings representing each prefix.
// Returns nil if the PrefixList is nil.
func (p *PrefixList) Slice() []string {
	return p.prefixesToStrings()
}
