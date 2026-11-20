package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
)

// The rpc provider talks to its consumer through bmclib's shared HTTP client, so it shows
// whether Connection.InsecureTLS reaches that client. httptest's certificate is not in the
// system trust store, so it can only be accepted when verification is skipped.
func TestTranslateInsecureTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tests := map[string]struct {
		insecureTLS bool
		wantErr     string
	}{
		"insecureTLS true skips verification":        {insecureTLS: true},
		"insecureTLS false verifies the certificate": {insecureTLS: false, wantErr: "certificate"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			opts := &controller.BMCOptions{
				ProviderOptions: &bmc.ProviderOptions{RPC: &bmc.RPCOptions{ConsumerURL: srv.URL}},
				InsecureTLS:     tt.insecureTLS,
			}
			cl := bmclib.NewClient("127.0.0.1", "", "", opts.Translate("127.0.0.1")...)
			cl.Registry.Drivers = cl.Registry.For("rpc")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := cl.Open(ctx)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("want an error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
