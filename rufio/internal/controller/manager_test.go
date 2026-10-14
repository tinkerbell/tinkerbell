package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// TestManagerClientReadsConfigMapsDirectly checks that, once the options are prepared, the
// manager's client reads a ConfigMap straight from the API server. A cached read would fail
// here: the manager is never started, so no informer ever syncs.
func TestManagerClientReadsConfigMapsDirectly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`))
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["get"]}]}`))
	})
	mux.HandleFunc("/api/v1/namespaces/default/configmaps/test-cert", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"ConfigMap","apiVersion":"v1","metadata":{"namespace":"default","name":"test-cert"},"data":{"cert.pem":"value"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts := ctrl.Options{
		Scheme:                 controller.DefaultScheme(),
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	}
	controller.DisableConfigMapCache(&opts)

	mgr, err := ctrl.NewManager(&rest.Config{Host: srv.URL}, opts)
	if err != nil {
		t.Fatalf("creating manager: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-cert"}, cm); err != nil {
		t.Fatalf("expected a direct read of the ConfigMap, got: %v", err)
	}
	if got := cm.Data["cert.pem"]; got != "value" {
		t.Fatalf("expected cert.pem %q, got %q", "value", got)
	}
}
