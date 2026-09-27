package reservation

import (
	"context"
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
)

func TestSetDHCPOptsDNSDefaults(t *testing.T) {
	hardwareDNS := []net.IP{net.ParseIP("192.0.2.53")}
	hardwareDNSv6 := net.ParseIP("2001:db8::53")
	defaultDNS := []net.IP{net.ParseIP("192.0.2.54")}

	tests := map[string]struct {
		hardware         *dhcp.DHCP
		wantNameServers  []string
		wantDomainSearch []string
	}{
		"Hardware settings win": {
			hardware:         &dhcp.DHCP{NameServers: hardwareDNS, DomainSearch: []string{"hw.example.com"}},
			wantNameServers:  []string{"192.0.2.53"},
			wantDomainSearch: []string{"hw.example.com"},
		},
		"defaults fill the gap": {
			hardware:         &dhcp.DHCP{},
			wantNameServers:  []string{"192.0.2.54"},
			wantDomainSearch: []string{"default.example.com"},
		},
		"IPv6-only Hardware nameservers fall back": {
			hardware:         &dhcp.DHCP{NameServers: []net.IP{hardwareDNSv6}},
			wantNameServers:  []string{"192.0.2.54"},
			wantDomainSearch: []string{"default.example.com"},
		},
		"mixed-family Hardware nameservers keep IPv4 only": {
			hardware:         &dhcp.DHCP{NameServers: []net.IP{hardwareDNS[0], hardwareDNSv6}},
			wantNameServers:  []string{"192.0.2.53"},
			wantDomainSearch: []string{"default.example.com"},
		},
		"each option falls back independently": {
			hardware:         &dhcp.DHCP{NameServers: hardwareDNS},
			wantNameServers:  []string{"192.0.2.53"},
			wantDomainSearch: []string{"default.example.com"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := &Handler{
				Log: logr.Discard(),
				DNSDefaults: DNSDefaults{
					NameServers:  defaultDNS,
					DomainSearch: []string{"default.example.com"},
				},
			}

			reply, err := dhcpv4.New(dhcpv4.PrependModifiers(h.setDHCPOpts(context.Background(), nil, tt.hardware))...)
			if err != nil {
				t.Fatal(err)
			}

			var gotNameServers []string
			for _, ns := range reply.DNS() {
				gotNameServers = append(gotNameServers, ns.String())
			}
			if diff := cmp.Diff(tt.wantNameServers, gotNameServers); diff != "" {
				t.Errorf("name servers mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantDomainSearch, reply.DomainSearch().Labels); diff != "" {
				t.Errorf("domain search mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
