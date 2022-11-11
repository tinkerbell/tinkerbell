package request

import (
	"net"
	"net/http"
)

// RemoteAddrIP retrieves the remote address IP from r.
func RemoteAddrIP(r *http.Request) (string, error) {
	addr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}
	return addr, nil
}
