package xff

import (
	"net"
	"net/http"
	"strings"

	"github.com/packethost/xff"
	"github.com/pkg/errors"
)

func ParseTrustedProxies(trustedProxies string) []string {
	var result []string

	for _, cidr := range strings.Split(trustedProxies, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			// Its not a cidr, but maybe its an IP
			if ip := net.ParseIP(cidr); ip != nil {
				if ip.To4() != nil {
					cidr += "/32"
				} else {
					cidr += "/128"
				}
			} else {
				// not an IP, panic
				panic("invalid ip cidr in TRUSTED_PROXIES cidr=" + cidr)
			}
		}
		result = append(result, cidr)
	}
	return result
}

// HTTPHandler creates a XFF handler if there are allowedSubnets specified.
func HTTPHandler(handler http.Handler, allowedSubnets []string) (http.Handler, error) {
	if len(allowedSubnets) == 0 {
		return handler, nil
	}

	xffmw, err := xff.New(xff.Options{
		AllowedSubnets: allowedSubnets,
	})
	if err != nil {
		return nil, errors.Errorf("create forward for handler: %v", err)
	}

	return xffmw.Handler(handler), nil
}
