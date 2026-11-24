package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bmc-toolbox/bmclib/v2"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
)

// TestTranslateDisableEtagMatch powers off a fake Redfish System through a bmclib client built
// from Translate, and checks the If-Match header on the ComputerSystem.Reset request.
func TestTranslateDisableEtagMatch(t *testing.T) {
	const etag = `"1234"`

	tests := map[string]struct {
		disableEtagMatch bool
		wantIfMatch      string
	}{
		"etag match enabled by default": {wantIfMatch: etag},
		"etag match disabled":           {disableEtagMatch: true, wantIfMatch: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			bmcServer := newRedfishSystem(t, etag)

			u, err := url.Parse(bmcServer.server.URL)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				t.Fatal(err)
			}
			opts := controller.BMCOptions{ProviderOptions: &bmc.ProviderOptions{
				Redfish: &bmc.RedfishOptions{
					Port:             port,
					UseBasicAuth:     true,
					DisableEtagMatch: tt.disableEtagMatch,
				},
			}}

			o := append(opts.Translate(u.Hostname()), bmclib.WithLogger(logr.Discard()))
			cl := bmclib.NewClient(u.Hostname(), "user", "pass", o...)
			cl.Registry.Drivers = cl.Registry.For("gofish")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := cl.Open(ctx); err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer cl.Close(ctx)

			if _, err := cl.SetPowerState(ctx, "off"); err != nil {
				t.Fatalf("SetPowerState(off) error = %v", err)
			}

			ifMatch, called := bmcServer.resetIfMatch()
			if !called {
				t.Fatal("ComputerSystem.Reset was not called")
			}
			if ifMatch != tt.wantIfMatch {
				t.Errorf("If-Match = %q, want %q", ifMatch, tt.wantIfMatch)
			}
		})
	}
}

// redfishSystem is a Redfish service with one powered-on System whose ETag is etag. It records
// the If-Match header of the System's ComputerSystem.Reset request.
type redfishSystem struct {
	server *httptest.Server

	mu      sync.Mutex
	called  bool
	ifMatch string
}

func newRedfishSystem(t *testing.T, etag string) *redfishSystem {
	t.Helper()
	r := &redfishSystem{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{
			"@odata.id": "/redfish/v1/",
			"RedfishVersion": "1.17.0",
			"Systems": {"@odata.id": "/redfish/v1/Systems"}
		}`)
	})
	mux.HandleFunc("GET /redfish/v1/Systems", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{
			"@odata.id": "/redfish/v1/Systems",
			"Members": [{"@odata.id": "/redfish/v1/Systems/System_0"}],
			"Members@odata.count": 1
		}`)
	})
	mux.HandleFunc("GET /redfish/v1/Systems/System_0", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", etag)
		writeJSON(w, `{
			"@odata.id": "/redfish/v1/Systems/System_0",
			"Id": "System_0",
			"Name": "System_0",
			"PowerState": "On",
			"Actions": {
				"#ComputerSystem.Reset": {
					"target": "/redfish/v1/Systems/System_0/Actions/ComputerSystem.Reset",
					"ResetType@Redfish.AllowableValues": ["On", "ForceOff"]
				}
			}
		}`)
	})
	mux.HandleFunc("POST /redfish/v1/Systems/System_0/Actions/ComputerSystem.Reset", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.called = true
		r.ifMatch = req.Header.Get("If-Match")
		r.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	r.server = httptest.NewTLSServer(mux)
	t.Cleanup(r.server.Close)

	return r
}

func (r *redfishSystem) resetIfMatch() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.ifMatch, r.called
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// TestTranslateProviders checks that a Machine's providers are the only providers the bmclib
// client registers, so Open connects to no others.
func TestTranslateProviders(t *testing.T) {
	tests := map[string]struct {
		providers []bmc.ProviderName
		want      []string
	}{
		"no providers uses every provider": {
			want: bmclib.ProviderNames(),
		},
		"gofish only": {
			providers: []bmc.ProviderName{"gofish"},
			want:      []string{"gofish"},
		},
		"names are case insensitive": {
			providers: []bmc.ProviderName{"GoFish", "intelamt"},
			want:      []string{"gofish", "IntelAMT"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			opts := controller.BMCOptions{ProviderOptions: &bmc.ProviderOptions{Providers: tt.providers}}
			o := append(opts.Translate("127.0.0.1"), bmclib.WithIpmitoolPath(fakeIpmitool(t)))
			cl := bmclib.NewClient("127.0.0.1", "user", "pass", o...)

			var got []string
			for _, d := range cl.Registry.Drivers {
				got = append(got, d.Name)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("registered providers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// fakeIpmitool returns the path of an empty file, so bmclib registers its ipmitool provider
// without the ipmitool binary installed. It is never run.
func fakeIpmitool(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipmitool")
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}

	return path
}
