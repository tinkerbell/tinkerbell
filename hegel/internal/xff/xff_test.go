package xff_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/hegel/internal/ginutil"
	. "github.com/tinkerbell/hegel/internal/xff"
)

func TestParse(t *testing.T) {
	// Table test of parsable inputs.
	// Table test of non-parsable inputs.
	cases := []struct {
		Name    string
		Proxies string
		Parsed  []string
		Err     bool
	}{
		{
			Name:    "Single IPv4",
			Proxies: "192.178.1.1",
			Parsed:  []string{"192.178.1.1/32"},
		},
		{
			Name:    "Multiple IPv4s",
			Proxies: "192.178.1.1,192.178.1.2",
			Parsed:  []string{"192.178.1.1/32", "192.178.1.2/32"},
		},
		{
			Name:    "Single IPv6s",
			Proxies: "2001:db8:0:0:0:ff00:42:8329",
			Parsed:  []string{"2001:db8:0:0:0:ff00:42:8329/128"},
		},
		{
			Name:    "Multiple IPv6s",
			Proxies: "2001:db8:0:0:0:ff00:42:8329,2001:db8::ff00:42:8329",
			Parsed:  []string{"2001:db8:0:0:0:ff00:42:8329/128", "2001:db8::ff00:42:8329/128"},
		},
		{
			Name:    "Mixed IPv4-IPv6",
			Proxies: "2001:db8::ff00:42:8329,192.178.1.2",
			Parsed:  []string{"2001:db8::ff00:42:8329/128", "192.178.1.2/32"},
		},
		{
			Name:    "Single IPv4 CIDR",
			Proxies: "192.178.0.0/16",
			Parsed:  []string{"192.178.0.0/16"},
		},
		{
			Name:    "Multiple IPv4 CIDR",
			Proxies: "192.178.0.0/16,192.179.0.0/15",
			Parsed:  []string{"192.178.0.0/16", "192.179.0.0/15"},
		},
		{
			Name:    "Single IPv6 CIDR",
			Proxies: "2001:db8::ff00:42:8329/64",
			Parsed:  []string{"2001:db8::ff00:42:8329/64"},
		},
		{
			Name:    "Multiple IPv6 CIDR",
			Proxies: "2001:db8::ff00:42:8329/64,2001:db8::ffff:42:8329/50",
			Parsed:  []string{"2001:db8::ff00:42:8329/64", "2001:db8::ffff:42:8329/50"},
		},
		{
			Name:    "Mixed IP and CIDR",
			Proxies: "2001:db8::ff00:42:8329,192.179.0.0/15,192.178.1.2,2001:db8::ffff:42:8329/50",
			Parsed:  []string{"2001:db8::ff00:42:8329/128", "192.179.0.0/15", "192.178.1.2/32", "2001:db8::ffff:42:8329/50"},
		},
		{
			Name:    "Ignore whitespace 1",
			Proxies: "192.168.0.0/16, 192.168.0.1",
			Parsed:  []string{"192.168.0.0/16", "192.168.0.1/32"},
		},
		{
			Name:    "Ignore whitespace prefix",
			Proxies: " 192.168.0.0/16,192.168.0.1",
			Parsed:  []string{"192.168.0.0/16", "192.168.0.1/32"},
		},
		{
			Name:    "Ignore whitespace suffix",
			Proxies: " 192.168.0.0/16,192.168.0.1",
			Parsed:  []string{"192.168.0.0/16", "192.168.0.1/32"},
		},
		{
			Name:    "Ignore empty entry",
			Proxies: " 192.168.0.0/16,, ,192.168.0.1",
			Parsed:  []string{"192.168.0.0/16", "192.168.0.1/32"},
		},

		// Error cases.
		{
			Name:    "Invalid IPv4",
			Proxies: "256.256.256.256",
			Err:     true,
		},
		{
			Name:    "Invalid CIDR",
			Proxies: "128.128.128.0/256",
			Err:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			parsed, err := Parse(tc.Proxies)

			// Check if we expect an error but got none.
			if tc.Err && err == nil {
				t.Fatal("expected error, got nil")
			}

			if !cmp.Equal(parsed, tc.Parsed) {
				t.Fatalf("parsed mismatch: %v", cmp.Diff(parsed, tc.Parsed))
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	cases := []struct {
		Name               string
		AllowedSubnets     []string
		RemoteAddr         string
		XFFAddr            string
		ExpectedRemoteAddr string
		Err                bool
	}{
		{
			Name:               "XFF with trusted range",
			AllowedSubnets:     []string{"192.168.0.0/16"},
			RemoteAddr:         "192.168.0.1:0",
			XFFAddr:            "10.10.10.10",
			ExpectedRemoteAddr: "10.10.10.10:0",
		},
		{
			Name:               "XFF with single trusted IP",
			AllowedSubnets:     []string{"192.168.0.1/32"},
			RemoteAddr:         "192.168.0.1:0",
			XFFAddr:            "10.10.10.10",
			ExpectedRemoteAddr: "10.10.10.10:0",
		},
		{
			Name:               "XFF without trusted IP",
			AllowedSubnets:     []string{"192.168.0.0/16"},
			RemoteAddr:         "192.178.0.1:0",
			XFFAddr:            "10.10.10.10",
			ExpectedRemoteAddr: "192.178.0.1:0",
		},
		{
			Name:               "No XFF with trusted range",
			AllowedSubnets:     []string{"192.168.0.0/16"},
			RemoteAddr:         "192.168.0.1:0",
			ExpectedRemoteAddr: "192.168.0.1:0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			// Build the request with the XFF header if specified.
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.RemoteAddr
			if tc.XFFAddr != "" {
				req.Header.Set("X-Forwarded-For", tc.XFFAddr)
			}

			w := ginutil.FakeResponseWriter{ResponseRecorder: httptest.NewRecorder()}

			ctx := &gin.Context{
				Request: req,
				Writer:  w,
			}

			// Build the middleware.
			mw, err := Middleware(tc.AllowedSubnets)
			if err != nil {
				t.Fatal(err)
			}

			// Serve and check the results.
			mw(ctx)

			if w.Code != http.StatusOK {
				t.Fatalf("unexpected status code: %d", w.Code)
			}

			// The request is passed through as a pointer so we can check the original request.
			if req.RemoteAddr != tc.ExpectedRemoteAddr {
				t.Fatalf(
					"unexpected remote addr: got %s, want %s",
					req.RemoteAddr,
					tc.ExpectedRemoteAddr,
				)
			}
		})
	}
}

func TestMiddlewareInvalidSubnets(t *testing.T) {
	cases := []string{
		"dsadsa",
		"256.256.256.256/16",
		"192.168.0.0/33",
	}

	for i, subnet := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			_, err := Middleware([]string{subnet})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
