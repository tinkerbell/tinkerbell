package netip

import (
	"fmt"
	"net/netip"
	"strings"
)

type AddrPort struct{ *netip.AddrPort }

func (a *AddrPort) Set(s string) error {
	if s == "" {
		return nil
	}

	ip, err := netip.ParseAddrPort(strings.TrimSpace(s))
	if !ip.IsValid() || err != nil {
		println("failed to parse Addr:Port:", a.String())
		return fmt.Errorf("failed to parse Addr:Port: %q: err: %v", s, err)
	}
	*a.AddrPort = ip

	return nil
}

func (a *AddrPort) Reset() error {
	*a.AddrPort = netip.AddrPort{}

	return nil
}

func (a *AddrPort) Type() string {
	return "addr:port"
}

type Addr struct{ *netip.Addr }

func (a *Addr) Set(s string) error {
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

func (a *Addr) Reset() error {
	*a.Addr = netip.Addr{}

	return nil
}

func (a *Addr) Type() string {
	return "addr"
}

type Prefix struct{ *netip.Prefix }

func (p *Prefix) Set(s string) error {
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

func (p *Prefix) Type() string {
	return "prefix"
}

type PrefixList struct {
	PrefixList *[]netip.Prefix
}

func (p *PrefixList) Set(s string) error {
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

func (p *PrefixList) Reset() error {
	*p.PrefixList = nil

	return nil
}

func (p *PrefixList) Type() string {
	return "prefix list"
}

func ToPrefixList(p *[]netip.Prefix) *PrefixList {
	pl := PrefixList{PrefixList: p}

	return &pl
}

func (p *PrefixList) String() string {
	var pl []string
	for _, prefix := range *p.PrefixList {
		pl = append(pl, prefix.String())
	}

	return strings.Join(pl, ",")
}

func (p *PrefixList) Slice() []string {
	var pl []string
	for _, prefix := range *p.PrefixList {
		pl = append(pl, prefix.String())
	}

	return pl
}
