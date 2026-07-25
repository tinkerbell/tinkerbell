package binary

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/smee/internal/hardware"
)

// pxeHTTPRouter builds a Router with the same routes PXEHTTPHandler enables:
// pxelinux config lookup by MAC, then disk asset fall-through.
func pxeHTTPRouter(resolver hardware.Resolver, dir string) Router {
	return Router{
		Log: logr.Discard(),
		Routes: []Route{
			PXELinuxMACRoute{Log: logr.Discard(), Resolver: resolver},
			DiskAssetRoute{Log: logr.Discard(), Dir: dir},
		},
	}
}

func TestHTTPHandler(t *testing.T) {
	mac, err := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatal(err)
	}
	const pxeConfig = "PROMPT 0\nDEFAULT linux"
	resolver := &fakeResolver{byMAC: map[string]hardware.Info{
		mac.String(): {PXELINUX: hardware.PXELINUX{Config: pxeConfig}},
	}}

	dir := t.TempDir()
	const assetBody = "kernel-bytes"
	if err := os.WriteFile(filepath.Join(dir, "vmlinuz"), []byte(assetBody), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		method        string
		prefix        string
		target        string
		wantStatus    int
		wantBody      string
		wantLen       string
		wantEmptyBody bool
	}{
		"pxelinux config by mac": {
			method:     http.MethodGet,
			prefix:     "/tftp/",
			target:     "/tftp/pxelinux.cfg/01-aa-bb-cc-dd-ee-ff",
			wantStatus: http.StatusOK,
			wantBody:   pxeConfig,
			wantLen:    "22",
		},
		"disk asset": {
			method:     http.MethodGet,
			prefix:     "/tftp/",
			target:     "/tftp/vmlinuz",
			wantStatus: http.StatusOK,
			wantBody:   assetBody,
			wantLen:    "12",
		},
		"custom prefix stripped": {
			method:     http.MethodGet,
			prefix:     "/boot/",
			target:     "/boot/vmlinuz",
			wantStatus: http.StatusOK,
			wantBody:   assetBody,
			wantLen:    "12",
		},
		"prefix without leading slash still strips": {
			method:     http.MethodGet,
			prefix:     "tftp",
			target:     "/tftp/vmlinuz",
			wantStatus: http.StatusOK,
			wantBody:   assetBody,
			wantLen:    "12",
		},
		"prefix with repeated slashes still strips": {
			method:     http.MethodGet,
			prefix:     "/tftp//",
			target:     "/tftp/vmlinuz",
			wantStatus: http.StatusOK,
			wantBody:   assetBody,
			wantLen:    "12",
		},
		"head returns length without body": {
			method:        http.MethodHead,
			prefix:        "/tftp/",
			target:        "/tftp/vmlinuz",
			wantStatus:    http.StatusOK,
			wantLen:       "12",
			wantEmptyBody: true,
		},
		"unknown path is 404": {
			method:     http.MethodGet,
			prefix:     "/tftp/",
			target:     "/tftp/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
		"method not allowed": {
			method:     http.MethodPost,
			prefix:     "/tftp/",
			target:     "/tftp/vmlinuz",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := HTTPHandler{
				Log:        logr.Discard(),
				Router:     pxeHTTPRouter(resolver, dir),
				PathPrefix: tt.prefix,
			}
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rr := httptest.NewRecorder()
			h.Handle(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
			if tt.wantEmptyBody && rr.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rr.Body.String())
			}
			if tt.wantLen != "" {
				if got := rr.Header().Get("Content-Length"); got != tt.wantLen {
					t.Errorf("Content-Length = %q, want %q", got, tt.wantLen)
				}
			}
		})
	}
}
