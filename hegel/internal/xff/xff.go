package xff

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/pkg/xff"
)

// Parse parses a string of comma separated trusted proxies. A trusted proxy can be a CIDR or an IP.
// IPs are converetd to CIDR notation with /32 or /128 for IPv4 and IPv6 respectively.
//
// Parse formats proxies appropriate for use with Middleware.
func Parse(trustedProxies string) ([]string, error) {
	var result []string

	for _, cidr := range strings.Split(trustedProxies, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}

		_, _, err := net.ParseCIDR(cidr)
		if err == nil {
			result = append(result, cidr)
			continue
		}

		// Its not a cidr, but maybe its an IP
		if ip := net.ParseIP(cidr); ip != nil {
			if ip.To4() != nil {
				cidr += "/32"
			} else {
				cidr += "/128"
			}

			result = append(result, cidr)
			continue
		}

		return nil, fmt.Errorf("invalid cidr or ip: %v", cidr)
	}

	return result, nil
}

// Middleware creates an X-Forward-For middlware in the form of an http.Handler. The middleware
// will replace the http.Request.RemoteAddr with the X-Forward-For header address if the
// http.Request.RemoteAddr is in allowedSubnets. It then calls handler with the newly configured
// http.Request.
//
// allowedSubnets is a slice of CIDR blocks. Individual IPs should be formatted with /32 or /128
// for IPv4 and IPv6 respectively.
func Middleware(proxies []string) (gin.HandlerFunc, error) {
	if len(proxies) == 0 {
		return func(_ *gin.Context) {}, nil
	}

	xffmw, err := xff.NewXFF(xff.Options{AllowedSubnets: proxies})
	if err != nil {
		return nil, fmt.Errorf("create forward for handler: %w", err)
	}

	// The upstream xff package doesn't support Gin so we need to leverage what it does provide
	// to create a Gin compatible middleware. The ServeHTTP satisfies a different framework
	// but is the clearest way to call the xffmw while honoring expected Gin behavior.
	//
	// When we separate from packethost packages we can tidy this up with our own implementation.
	return func(ctx *gin.Context) {
		hl := xffmw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			// Given we're a Gin middleware we need to call the next handler in the chain.
			ctx.Next()
		}))
		hl.ServeHTTP(ctx.Writer, ctx.Request)
	}, nil
}

// MiddlewareFromUnparsed is a helpe that calls Parse then Middleware. proxies must conform to the
// Parse constraints.
func MiddlewareFromUnparsed(proxies string) (gin.HandlerFunc, error) {
	parsed, err := Parse(proxies)
	if err != nil {
		return nil, err
	}

	return Middleware(parsed)
}
