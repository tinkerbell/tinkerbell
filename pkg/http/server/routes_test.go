package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

// dummyHandler is a simple handler that writes a known body so tests can
// distinguish between receiving the real handler vs. a redirect.
func dummyHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, body)
	})
}

func TestRegister_Defaults(t *testing.T) {
	var rs Routes
	h := dummyHandler("ok")
	rs.Register("/test", h, "test route")

	if len(rs) != 1 {
		t.Fatalf("got %d routes, want 1", len(rs))
	}
	r := rs[0]
	if r.Pattern != "/test" {
		t.Errorf("Pattern = %q, want %q", r.Pattern, "/test")
	}
	if r.Description != "test route" {
		t.Errorf("Description = %q, want %q", r.Description, "test route")
	}
	if !r.HTTPEnabled {
		t.Error("HTTPEnabled = false, want true (default)")
	}
	if r.HTTPSEnabled {
		t.Error("HTTPSEnabled = true, want false (default)")
	}
	if r.RewriteHTTPToHTTPS {
		t.Error("RewriteHTTPToHTTPS = true, want false (default)")
	}
}

func TestRegister_EmptyDescription(t *testing.T) {
	var rs Routes
	rs.Register("/x", dummyHandler("x"), "")
	if rs[0].Description != "No description provided" {
		t.Errorf("Description = %q, want fallback", rs[0].Description)
	}
}

func TestRegister_WithOptions(t *testing.T) {
	var rs Routes
	rs.Register("/secure", dummyHandler("s"), "secure",
		WithHTTPEnabled(false),
		WithHTTPSEnabled(true),
		WithRewriteHTTPToHTTPS(true),
	)

	r := rs[0]
	if r.HTTPEnabled {
		t.Error("HTTPEnabled = true, want false")
	}
	if !r.HTTPSEnabled {
		t.Error("HTTPSEnabled = false, want true")
	}
	if !r.RewriteHTTPToHTTPS {
		t.Error("RewriteHTTPToHTTPS = false, want true")
	}
}

func TestMuxes(t *testing.T) {
	const httpsPort = 7443

	tests := []struct {
		name string
		// Route options
		httpEnabled        bool
		httpsEnabled       bool
		rewriteHTTPToHTTPS bool
		// Muxes parameter
		tlsEnabled bool
		// Expectations: whether the HTTP mux serves handler, redirect, or 404
		wantHTTPHandler  bool // true → handler body; false → either redirect or 404
		wantHTTPRedirect bool // true → 308 redirect to HTTPS
		// Whether the HTTPS mux serves the handler
		wantHTTPSHandler bool
	}{
		{
			name:             "http only, no TLS",
			httpEnabled:      true,
			httpsEnabled:     false,
			tlsEnabled:       false,
			wantHTTPHandler:  true,
			wantHTTPSHandler: false,
		},
		{
			name:             "http and https, no TLS",
			httpEnabled:      true,
			httpsEnabled:     true,
			tlsEnabled:       false,
			wantHTTPHandler:  true,
			wantHTTPSHandler: true,
		},
		{
			name:               "rewrite requested but TLS disabled: handler served on HTTP",
			httpEnabled:        true,
			httpsEnabled:       true,
			rewriteHTTPToHTTPS: true,
			tlsEnabled:         false,
			wantHTTPHandler:    true,
			wantHTTPRedirect:   false,
			wantHTTPSHandler:   true,
		},
		{
			name:               "rewrite with TLS enabled: redirect on HTTP, handler on HTTPS",
			httpEnabled:        true,
			httpsEnabled:       true,
			rewriteHTTPToHTTPS: true,
			tlsEnabled:         true,
			wantHTTPHandler:    false,
			wantHTTPRedirect:   true,
			wantHTTPSHandler:   true,
		},
		{
			name:             "https only",
			httpEnabled:      false,
			httpsEnabled:     true,
			tlsEnabled:       true,
			wantHTTPHandler:  false,
			wantHTTPSHandler: true,
		},
		{
			name:             "neither http nor https",
			httpEnabled:      false,
			httpsEnabled:     false,
			tlsEnabled:       false,
			wantHTTPHandler:  false,
			wantHTTPSHandler: false,
		},
		{
			name:             "http only with TLS enabled",
			httpEnabled:      true,
			httpsEnabled:     false,
			tlsEnabled:       true,
			wantHTTPHandler:  true,
			wantHTTPSHandler: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const body = "handler-response"
			var rs Routes
			var opts []func(*Route)
			opts = append(opts, WithHTTPEnabled(tt.httpEnabled))
			opts = append(opts, WithHTTPSEnabled(tt.httpsEnabled))
			if tt.rewriteHTTPToHTTPS {
				opts = append(opts, WithRewriteHTTPToHTTPS(true))
			}
			rs.Register("/test", dummyHandler(body), "test", opts...)

			httpMux, httpsMux := rs.Muxes(logr.Discard(), httpsPort, tt.tlsEnabled)

			// --- HTTP mux ---
			httpReq := httptest.NewRequest(http.MethodGet, "/test", nil)
			httpReq.Host = "example.com:7080"
			httpRec := httptest.NewRecorder()
			httpMux.ServeHTTP(httpRec, httpReq)

			switch {
			case tt.wantHTTPHandler:
				if httpRec.Code != http.StatusOK {
					t.Errorf("HTTP mux: status = %d, want %d", httpRec.Code, http.StatusOK)
				}
				if httpRec.Body.String() != body {
					t.Errorf("HTTP mux: body = %q, want %q", httpRec.Body.String(), body)
				}
			case tt.wantHTTPRedirect:
				if httpRec.Code != http.StatusPermanentRedirect {
					t.Errorf("HTTP mux: status = %d, want %d (redirect)", httpRec.Code, http.StatusPermanentRedirect)
				}
				loc := httpRec.Header().Get("Location")
				wantLoc := fmt.Sprintf("https://example.com:%d/test", httpsPort)
				if loc != wantLoc {
					t.Errorf("HTTP mux: Location = %q, want %q", loc, wantLoc)
				}
			case tt.httpEnabled:
				// httpEnabled but no handler/redirect expected is contradictory;
				// this branch shouldn't be reached with valid test data.
				t.Fatal("unexpected: httpEnabled=true but neither handler nor redirect expected")
			default:
				// Route not registered on HTTP mux → expect 404.
				if httpRec.Code != http.StatusNotFound {
					t.Errorf("HTTP mux: status = %d, want %d (not found)", httpRec.Code, http.StatusNotFound)
				}
			}

			// --- HTTPS mux ---
			httpsReq := httptest.NewRequest(http.MethodGet, "/test", nil)
			httpsReq.Host = "example.com:7443"
			httpsRec := httptest.NewRecorder()
			httpsMux.ServeHTTP(httpsRec, httpsReq)

			if tt.wantHTTPSHandler {
				if httpsRec.Code != http.StatusOK {
					t.Errorf("HTTPS mux: status = %d, want %d", httpsRec.Code, http.StatusOK)
				}
				if httpsRec.Body.String() != body {
					t.Errorf("HTTPS mux: body = %q, want %q", httpsRec.Body.String(), body)
				}
			} else if httpsRec.Code != http.StatusNotFound {
				t.Errorf("HTTPS mux: status = %d, want %d (not found)", httpsRec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHasHTTPSRoutes(t *testing.T) {
	var rs Routes
	if rs.HasHTTPSRoutes() {
		t.Error("empty Routes: HasHTTPSRoutes = true, want false")
	}

	rs.Register("/a", dummyHandler("a"), "a")
	if rs.HasHTTPSRoutes() {
		t.Error("HTTP-only route: HasHTTPSRoutes = true, want false")
	}

	rs.Register("/b", dummyHandler("b"), "b", WithHTTPSEnabled(true))
	if !rs.HasHTTPSRoutes() {
		t.Error("after HTTPS route: HasHTTPSRoutes = false, want true")
	}
}

func TestMuxes_MultipleRoutes(t *testing.T) {
	var rs Routes
	rs.Register("/public", dummyHandler("public"), "public route")
	rs.Register("/secure", dummyHandler("secure"), "secure route",
		WithHTTPEnabled(true),
		WithHTTPSEnabled(true),
		WithRewriteHTTPToHTTPS(true),
	)
	rs.Register("/https-only", dummyHandler("https-only"), "https only",
		WithHTTPEnabled(false),
		WithHTTPSEnabled(true),
	)

	httpMux, httpsMux := rs.Muxes(logr.Discard(), 7443, true)

	// /public should be served on HTTP mux.
	rec := httptest.NewRecorder()
	httpMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "public" {
		t.Errorf("/public on HTTP: status=%d body=%q", rec.Code, rec.Body.String())
	}

	// /secure should redirect on HTTP mux (TLS enabled + rewrite).
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Host = "example.com:7080"
	httpMux.ServeHTTP(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Errorf("/secure on HTTP: status=%d, want %d", rec.Code, http.StatusPermanentRedirect)
	}

	// /secure should be served on HTTPS mux.
	rec = httptest.NewRecorder()
	httpsMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secure", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "secure" {
		t.Errorf("/secure on HTTPS: status=%d body=%q", rec.Code, rec.Body.String())
	}

	// /https-only should 404 on HTTP mux.
	rec = httptest.NewRecorder()
	httpMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/https-only", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("/https-only on HTTP: status=%d, want 404", rec.Code)
	}

	// /https-only should be served on HTTPS mux.
	rec = httptest.NewRecorder()
	httpsMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/https-only", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "https-only" {
		t.Errorf("/https-only on HTTPS: status=%d body=%q", rec.Code, rec.Body.String())
	}
}
