package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newWebhookTestClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToSchemeTinkerbell(scheme)
	_ = api.AddToSchemeBMC(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestTemplateWebhook(t *testing.T) {
	hw := &v1alpha1.Hardware{
		Spec: v1alpha1.HardwareSpec{
			Interfaces: []v1alpha1.Interface{
				{DHCP: &v1alpha1.DHCP{MAC: "aa:bb:cc:dd:ee:ff"}},
			},
		},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-token", Namespace: "default"},
		Data:       map[string][]byte{"bearer-token": []byte("Bearer abc123")},
	}
	basicAuthSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-creds", Namespace: "default"},
		Data:       map[string][]byte{"username": []byte("svc"), "password": []byte("s3cr3t")},
	}
	incompleteBasicAuthSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "incomplete-creds", Namespace: "default"},
		Data:       map[string][]byte{"username": []byte("svc")},
	}

	tests := map[string]struct {
		wh        v1alpha1.WebhookAction
		hw        *v1alpha1.Hardware
		objs      []client.Object
		wantErr   bool
		wantURL   string
		wantBody  string
		wantHdrs  map[string]string
		wantBasic bool
		wantUser  string
		wantPass  string
	}{
		"templates URL and body from hardware": {
			wh: v1alpha1.WebhookAction{
				URL:  "https://example.com/{{ (index .Hardware.Interfaces 0).DHCP.MAC }}",
				Body: "mac={{ (index .Hardware.Interfaces 0).DHCP.MAC }}",
			},
			hw:       hw,
			wantURL:  "https://example.com/aa:bb:cc:dd:ee:ff",
			wantBody: "mac=aa:bb:cc:dd:ee:ff",
			wantHdrs: map[string]string{},
		},
		"static header value is templated": {
			wh: v1alpha1.WebhookAction{
				URL: "https://example.com",
				Headers: []v1alpha1.WebhookHeader{
					{Name: "X-MAC", Value: "{{ (index .Hardware.Interfaces 0).DHCP.MAC }}"},
				},
			},
			hw:       hw,
			wantURL:  "https://example.com",
			wantHdrs: map[string]string{"X-MAC": "aa:bb:cc:dd:ee:ff"},
		},
		"header valueFrom resolves secret key": {
			wh: v1alpha1.WebhookAction{
				URL: "https://example.com",
				Headers: []v1alpha1.WebhookHeader{
					{Name: "Authorization", ValueFrom: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "webhook-token"},
						Key:                  "bearer-token",
					}},
				},
			},
			hw:       hw,
			objs:     []client.Object{tokenSecret},
			wantURL:  "https://example.com",
			wantHdrs: map[string]string{"Authorization": "Bearer abc123"},
		},
		"header valueFrom missing secret errors": {
			wh: v1alpha1.WebhookAction{
				URL: "https://example.com",
				Headers: []v1alpha1.WebhookHeader{
					{Name: "Authorization", ValueFrom: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "does-not-exist"},
						Key:                  "bearer-token",
					}},
				},
			},
			hw:      hw,
			wantErr: true,
		},
		"header valueFrom missing key errors": {
			wh: v1alpha1.WebhookAction{
				URL: "https://example.com",
				Headers: []v1alpha1.WebhookHeader{
					{Name: "Authorization", ValueFrom: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "webhook-token"},
						Key:                  "does-not-exist",
					}},
				},
			},
			hw:      hw,
			objs:    []client.Object{tokenSecret},
			wantErr: true,
		},
		"basicAuth resolves username and password": {
			wh: v1alpha1.WebhookAction{
				URL:       "https://example.com",
				BasicAuth: &corev1.SecretReference{Name: "webhook-creds", Namespace: "default"},
			},
			hw:        hw,
			objs:      []client.Object{basicAuthSecret},
			wantURL:   "https://example.com",
			wantHdrs:  map[string]string{},
			wantBasic: true,
			wantUser:  "svc",
			wantPass:  "s3cr3t",
		},
		"basicAuth missing password key errors": {
			wh: v1alpha1.WebhookAction{
				URL:       "https://example.com",
				BasicAuth: &corev1.SecretReference{Name: "incomplete-creds", Namespace: "default"},
			},
			hw:      hw,
			objs:    []client.Object{incompleteBasicAuthSecret},
			wantErr: true,
		},
		"nil hardware is tolerated": {
			wh:       v1alpha1.WebhookAction{URL: "https://example.com/static"},
			hw:       nil,
			wantURL:  "https://example.com/static",
			wantHdrs: map[string]string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := newWebhookTestClient(tc.objs...)
			rw, err := templateWebhook(context.Background(), c, "default", tc.wh, tc.hw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error: %v, got: %v", tc.wantErr, err)
			}
			if tc.wantErr {
				return
			}
			if rw.URL != tc.wantURL {
				t.Errorf("URL: want %q, got %q", tc.wantURL, rw.URL)
			}
			if tc.wantBody != "" && rw.Body != tc.wantBody {
				t.Errorf("Body: want %q, got %q", tc.wantBody, rw.Body)
			}
			if diff := cmp.Diff(tc.wantHdrs, rw.Headers); tc.wantHdrs != nil && diff != "" {
				t.Errorf("Headers (-want +got):\n%s", diff)
			}
			if rw.HasBasicAuth != tc.wantBasic {
				t.Errorf("HasBasicAuth: want %v, got %v", tc.wantBasic, rw.HasBasicAuth)
			}
			if tc.wantBasic {
				if rw.BasicAuthUser != tc.wantUser || rw.BasicAuthPass != tc.wantPass {
					t.Errorf("BasicAuth: want %s/%s, got %s/%s", tc.wantUser, tc.wantPass, rw.BasicAuthUser, rw.BasicAuthPass)
				}
			}
		})
	}
}

func TestCallWebhook(t *testing.T) {
	tests := map[string]struct {
		rw          resolvedWebhook
		handler     http.HandlerFunc
		wantErr     bool
		wantMethod  string
		wantUser    string
		wantPass    string
		wantHasAuth bool
	}{
		"defaults to POST and treats 2xx as success": {
			rw:         resolvedWebhook{Method: "", Body: "hi"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) },
			wantMethod: http.MethodPost,
		},
		"non-2xx without ExpectStatus is an error": {
			rw:      resolvedWebhook{},
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: true,
		},
		"ExpectStatus match succeeds even outside 2xx": {
			rw:      resolvedWebhook{ExpectStatus: http.StatusNotModified},
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotModified) },
		},
		"ExpectStatus mismatch is an error": {
			rw:      resolvedWebhook{ExpectStatus: http.StatusOK},
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) },
			wantErr: true,
		},
		"custom method is used": {
			rw:         resolvedWebhook{Method: http.MethodPut},
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			wantMethod: http.MethodPut,
		},
		"basicAuth sets the Authorization header via SetBasicAuth": {
			rw:          resolvedWebhook{HasBasicAuth: true, BasicAuthUser: "svc", BasicAuthPass: "s3cr3t"},
			wantHasAuth: true,
			wantUser:    "svc",
			wantPass:    "s3cr3t",
			handler: func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "svc" || pass != "s3cr3t" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
		"basicAuth overwrites a manually-set Authorization header": {
			rw: resolvedWebhook{
				Headers:       map[string]string{"Authorization": "Bearer wrong-token"},
				HasBasicAuth:  true,
				BasicAuthUser: "svc",
				BasicAuthPass: "s3cr3t",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "svc" || pass != "s3cr3t" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				tc.handler(w, r)
			}))
			defer srv.Close()

			rw := tc.rw
			rw.URL = srv.URL
			s := &state{}
			err := s.callWebhook(context.Background(), rw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error: %v, got: %v", tc.wantErr, err)
			}
			if tc.wantMethod != "" && gotMethod != tc.wantMethod {
				t.Errorf("method: want %s, got %s", tc.wantMethod, gotMethod)
			}
		})
	}
}
