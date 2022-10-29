package xff

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/packethost/xff"
	"github.com/pkg/errors"
)

// Parse parses a string of comma separated trusted proxies. A trusted proxy can be a CIDR or an IP.
func Parse(trustedProxies string) ([]string, error) {
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
				return nil, fmt.Errorf("invalid cidr or ip: %v", cidr)
			}
		}
		result = append(result, cidr)
	}

	return result, nil
}

// Middleware creates an X-Forward-For middlware in the form of an an http.Handler. The middleware
// will replace the http.Request.RemoteAddr with the X-Forward-For header address if the
// http.Request.RemoteAddr is in the allowedSubnets. It then calls handler with the newly configured
// http.Request.
func Middleware(handler http.Handler, allowedSubnets []string) (http.Handler, error) {
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
