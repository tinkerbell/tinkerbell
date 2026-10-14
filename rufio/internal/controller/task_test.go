package controller_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func toPtr[T any](v T) *T {
	return &v
}

func getAction(s string) bmc.Action {
	switch s {
	case "PowerOn":
		return bmc.Action{PowerAction: toPtr(bmc.PowerOn)}
	case "HardOff":
		return bmc.Action{PowerAction: toPtr(bmc.PowerHardOff)}
	case "SoftOff":
		return bmc.Action{PowerAction: toPtr(bmc.PowerSoftOff)}
	case "BootPXE":
		return bmc.Action{OneTimeBootDeviceAction: &bmc.OneTimeBootDeviceAction{Devices: []bmc.BootDevice{bmc.PXE}}}
	case "VirtualMedia":
		return bmc.Action{VirtualMediaAction: &bmc.VirtualMediaAction{MediaURL: "http://example.com/image.iso", Kind: bmc.VirtualMediaCD}}
	case "HTTPBootEnabled":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootEnabled: toPtr(true)}}
	case "HTTPBootDisabled":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootEnabled: toPtr(false)}}
	case "PXEBootEnabled":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{PXEBootEnabled: toPtr(true)}}
	case "HTTPAndPXEBootEnabled":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootEnabled: toPtr(true), PXEBootEnabled: toPtr(true)}}
	case "HTTPBootURL":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootURL: toPtr("http://example.com/boot.efi")}}
	case "HTTPBootEnabledAndURL":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootEnabled: toPtr(true), HTTPBootURL: toPtr("http://example.com/boot.efi")}}
	case "HTTPBootTLSMode":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootTLSMode: toPtr("None")}}
	case "HTTPBootTLSModeAndURL":
		return bmc.Action{NetworkBootConfig: &bmc.NetworkBootConfig{HTTPBootTLSMode: toPtr("None"), HTTPBootURL: toPtr("http://example.com/boot.efi")}}
	default:
		return bmc.Action{}
	}
}

func TestTaskReconcile(t *testing.T) {
	tests := map[string]struct {
		taskName                 string
		action                   bmc.Action
		provider                 *testProvider
		secret                   *corev1.Secret
		configMap                *corev1.ConfigMap
		task                     *bmc.Task
		shouldErr                bool
		timeoutErr               bool
		wantNetworkBootCallOrder []string
	}{
		"success power on": {
			taskName: "PowerOn",
			action:   getAction("PowerOn"),
			provider: &testProvider{Powerstate: "on", PowerSetOK: true},
		},
		"success hard off": {
			taskName: "HardOff",
			action:   getAction("HardOff"),
			provider: &testProvider{Powerstate: "off", PowerSetOK: true},
		},
		"success soft off": {
			taskName: "SoftOff",
			action:   getAction("SoftOff"),
			provider: &testProvider{Powerstate: "off", PowerSetOK: true},
		},
		"success boot pxe": {
			taskName: "BootPXE",
			action:   getAction("BootPXE"),
			provider: &testProvider{BootdeviceOK: true},
		},
		"success virtual media": {
			taskName: "VirtualMedia",
			action:   getAction("VirtualMedia"),
			provider: &testProvider{VirtualMediaOK: true},
		},
		"success power on with rpc provider": {
			taskName: "PowerOn",
			action:   getAction("PowerOn"),
			provider: &testProvider{Powerstate: "on", PowerSetOK: true, Proto: "rpc"},
			secret:   createHMACSecret(),
			task:     createTaskWithRPC("PowerOn", getAction("PowerOn"), createHMACSecret()),
		},
		"success power on with RPC provider w/o secrets": {
			taskName: "PowerOn",
			action:   getAction("PowerOn"),
			provider: &testProvider{Powerstate: "on", PowerSetOK: true, Proto: "rpc"},
		},
		"failure on bmc open": {
			taskName: "PowerOn", action: getAction("PowerOn"),
			provider:  &testProvider{ErrOpen: errors.New("failed to open")},
			shouldErr: true,
		},
		"failure on bmc power on": {
			taskName:  "PowerOn",
			action:    getAction("PowerOn"),
			provider:  &testProvider{ErrPowerStateSet: errors.New("failed to set power state")},
			shouldErr: true,
		},
		"failure on set boot device": {
			taskName:  "BootPXE",
			action:    getAction("BootPXE"),
			provider:  &testProvider{ErrBootDeviceSet: errors.New("failed to set boot device")},
			shouldErr: true,
		},
		"failure on virtual media": {
			taskName:  "VirtualMedia",
			action:    getAction("VirtualMedia"),
			provider:  &testProvider{ErrVirtualMediaInsert: errors.New("failed to set virtual media")},
			shouldErr: true,
		},
		"failure timeout": {
			taskName:   "PowerOn",
			action:     getAction("PowerOn"),
			provider:   &testProvider{Powerstate: "off", PowerSetOK: true},
			timeoutErr: true,
		},
		"failure to find secret": {
			taskName:  "PowerOn",
			action:    getAction("PowerOn"),
			provider:  &testProvider{Powerstate: "off", PowerSetOK: true},
			secret:    &corev1.Secret{},
			task:      createTask("PowerOn", getAction("PowerOn"), &corev1.Secret{}),
			shouldErr: true,
		},
		"success secure boot enable": {
			taskName: "secure boot enable",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{Enable: toPtr(true)}},
			provider: &testProvider{},
		},
		"failure on secure boot enable": {
			taskName:  "secure boot enable",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{Enable: toPtr(true)}},
			provider:  &testProvider{ErrSetSecureBoot: errors.New("failed to set secure boot")},
			shouldErr: true,
		},
		"success secure boot disable": {
			taskName: "secure boot disable",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{Enable: toPtr(false)}},
			provider: &testProvider{SecureBootEnabled: true},
		},
		"failure on secure boot with no operation set": {
			taskName:  "secure boot empty",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{}},
			provider:  &testProvider{},
			shouldErr: true,
		},
		"success secure boot allow custom keys": {
			taskName: "secure boot allow custom keys",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{AllowCustomKeys: toPtr(true)}},
			provider: &testProvider{},
		},
		"failure on secure boot allow custom keys": {
			taskName:  "secure boot allow custom keys",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{AllowCustomKeys: toPtr(true)}},
			provider:  &testProvider{ErrAllowCustomSecureBootKeys: errors.New("failed to allow custom secure boot keys")},
			shouldErr: true,
		},
		"success secure boot allow custom keys disabled": {
			taskName: "secure boot disallow custom keys",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{AllowCustomKeys: toPtr(false)}},
			provider: &testProvider{},
		},
		"success secure boot delete PK": {
			taskName: "secure boot delete pk",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{ResetKeys: &bmc.SecureBootResetKeysAction{ResetType: "DeletePK"}}},
			provider: &testProvider{},
		},
		"success secure boot database reset keys KEK": {
			taskName: "secure boot database reset keys kek",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{DatabaseResetKeys: &bmc.SecureBootDatabaseResetKeysAction{Database: "KEK", ResetType: "DeleteAllKeys"}}},
			provider: &testProvider{},
		},
		"success secure boot certificate import KEK": {
			taskName:  "secure boot certificate import kek",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{CertificateImport: &bmc.SecureBootCertificateImportAction{Database: "KEK", CertificatePEMConfigMapRef: bmc.ConfigMapKeyReference{Name: "test-cert", Key: "cert.pem"}}}},
			provider:  &testProvider{},
			configMap: createCertConfigMap(t),
		},
		"success secure boot reset keys": {
			taskName: "secure boot reset keys",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{ResetKeys: &bmc.SecureBootResetKeysAction{ResetType: "DeleteAllKeys"}}},
			provider: &testProvider{},
		},
		"failure on secure boot reset keys": {
			taskName:  "secure boot reset keys",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{ResetKeys: &bmc.SecureBootResetKeysAction{ResetType: "DeleteAllKeys"}}},
			provider:  &testProvider{ErrResetSecureBootKeys: errors.New("failed to reset secure boot keys")},
			shouldErr: true,
		},
		"success secure boot database reset keys": {
			taskName: "secure boot database reset keys",
			action:   bmc.Action{SecureBoot: &bmc.SecureBootAction{DatabaseResetKeys: &bmc.SecureBootDatabaseResetKeysAction{Database: "db", ResetType: "ResetAllKeysToDefault"}}},
			provider: &testProvider{},
		},
		"failure on secure boot database reset keys": {
			taskName:  "secure boot database reset keys",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{DatabaseResetKeys: &bmc.SecureBootDatabaseResetKeysAction{Database: "db", ResetType: "ResetAllKeysToDefault"}}},
			provider:  &testProvider{ErrResetSecureBootDatabaseKeys: errors.New("failed to reset secure boot database keys")},
			shouldErr: true,
		},
		"success secure boot certificate import": {
			taskName:  "secure boot certificate import",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{CertificateImport: &bmc.SecureBootCertificateImportAction{Database: "db", CertificatePEMConfigMapRef: bmc.ConfigMapKeyReference{Name: "test-cert", Key: "cert.pem"}}}},
			provider:  &testProvider{},
			configMap: createCertConfigMap(t),
		},
		"failure on secure boot certificate import": {
			taskName:  "secure boot certificate import",
			action:    bmc.Action{SecureBoot: &bmc.SecureBootAction{CertificateImport: &bmc.SecureBootCertificateImportAction{Database: "db", CertificatePEMConfigMapRef: bmc.ConfigMapKeyReference{Name: "test-cert", Key: "cert.pem"}}}},
			provider:  &testProvider{ErrImportSecureBootCertificate: errors.New("failed to import secure boot certificate")},
			configMap: createCertConfigMap(t),
			shouldErr: true,
		},
		"success with boot device": {
			taskName: "boot device pxe",
			action: bmc.Action{
				BootDevice: &bmc.BootDeviceConfig{
					Device:     bmc.PXE,
					Persistent: true,
					EFIBoot:    true,
				},
			},
			provider: &testProvider{BootdeviceOK: true},
		},
		"failure on boot device set": {
			taskName: "boot device pxe",
			action: bmc.Action{
				BootDevice: &bmc.BootDeviceConfig{
					Device:     bmc.PXE,
					Persistent: true,
					EFIBoot:    true,
				},
			},
			provider:  &testProvider{ErrBootDeviceSet: errors.New("failed to set boot device")},
			shouldErr: true,
		},
		"failure to find task": {
			taskName:  "empty task",
			action:    bmc.Action{},
			provider:  &testProvider{},
			shouldErr: true,
		},
		"success http boot enabled": {
			taskName: "HTTPBootEnabled",
			action:   getAction("HTTPBootEnabled"),
			provider: &testProvider{NetworkBootEnabledOK: true},
		},
		"success http boot disabled": {
			taskName: "HTTPBootDisabled",
			action:   getAction("HTTPBootDisabled"),
			provider: &testProvider{NetworkBootEnabledOK: true},
		},
		"failure on http boot set network boot enabled error": {
			taskName:  "HTTPBootEnabled",
			action:    getAction("HTTPBootEnabled"),
			provider:  &testProvider{ErrSetNetworkBootEnabled: errors.New("failed to set network boot enabled state")},
			shouldErr: true,
		},
		"failure on http boot set network boot enabled not ok": {
			taskName:  "HTTPBootEnabled",
			action:    getAction("HTTPBootEnabled"),
			provider:  &testProvider{NetworkBootEnabledOK: false},
			shouldErr: true,
		},
		"success pxe boot enabled": {
			taskName: "PXEBootEnabled",
			action:   getAction("PXEBootEnabled"),
			provider: &testProvider{NetworkBootEnabledOK: true},
		},
		"success http and pxe boot both enabled": {
			taskName: "HTTPAndPXEBootEnabled",
			action:   getAction("HTTPAndPXEBootEnabled"),
			provider: &testProvider{NetworkBootEnabledOK: true},
		},
		"success http boot url": {
			taskName: "HTTPBootURL",
			action:   getAction("HTTPBootURL"),
			provider: &testProvider{HTTPBootURIOK: true},
		},
		"success http boot enabled and url set together": {
			taskName: "HTTPBootEnabledAndURL",
			action:   getAction("HTTPBootEnabledAndURL"),
			provider: &testProvider{NetworkBootEnabledOK: true, HTTPBootURIOK: true},
		},
		"failure on http boot url set error": {
			taskName:  "HTTPBootURL",
			action:    getAction("HTTPBootURL"),
			provider:  &testProvider{ErrHTTPBootURISet: errors.New("failed to set http boot uri")},
			shouldErr: true,
		},
		"failure on http boot url set not ok": {
			taskName:  "HTTPBootURL",
			action:    getAction("HTTPBootURL"),
			provider:  &testProvider{HTTPBootURIOK: false},
			shouldErr: true,
		},
		"success http boot tls mode": {
			taskName: "HTTPBootTLSMode",
			action:   getAction("HTTPBootTLSMode"),
			provider: &testProvider{HTTPBootTLSModeOK: true},
		},
		"failure on http boot tls mode set error": {
			taskName:  "HTTPBootTLSMode",
			action:    getAction("HTTPBootTLSMode"),
			provider:  &testProvider{ErrHTTPBootTLSModeSet: errors.New("failed to set http boot tls mode")},
			shouldErr: true,
		},
		"failure on http boot tls mode set not ok": {
			taskName:  "HTTPBootTLSMode",
			action:    getAction("HTTPBootTLSMode"),
			provider:  &testProvider{HTTPBootTLSModeOK: false},
			shouldErr: true,
		},
		"success http boot tls mode set before url": {
			taskName:                 "HTTPBootTLSModeAndURL",
			action:                   getAction("HTTPBootTLSModeAndURL"),
			provider:                 &testProvider{HTTPBootTLSModeOK: true, HTTPBootURIOK: true},
			wantNetworkBootCallOrder: []string{"SetHTTPBootTLSMode", "SetHTTPBootURI"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var secret *corev1.Secret
			if tt.secret != nil {
				secret = tt.secret
			} else {
				secret = createSecret()
			}
			var task *bmc.Task
			if tt.task != nil {
				task = tt.task
			} else {
				task = createTask(tt.taskName, tt.action, secret)
			}

			objs := []client.Object{task, secret}
			if tt.configMap != nil {
				objs = append(objs, tt.configMap)
			}
			cluster := newClientBuilder().
				WithObjects(objs...).
				Build()

			reconciler := controller.NewTaskReconciler(cluster, newTestClient(tt.provider))
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: task.Namespace,
					Name:      task.Name,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), request)
			if !tt.shouldErr && err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			if tt.shouldErr && err == nil {
				t.Fatalf("expected err, got: %v", err)
			}
			if tt.shouldErr {
				return
			}
			if diff := cmp.Diff(result, ctrl.Result{}); diff != "" {
				t.Fatalf("expected no diff, got: %v", diff)
			}
			if tt.wantNetworkBootCallOrder != nil {
				if diff := cmp.Diff(tt.provider.NetworkBootCallOrder, tt.wantNetworkBootCallOrder); diff != "" {
					t.Fatalf("unexpected NetworkBootConfig call order, got: %v", diff)
				}
			}

			var retrieved bmc.Task
			if err = cluster.Get(context.Background(), request.NamespacedName, &retrieved); err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			// TODO: g.Expect(retrieved.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			if !retrieved.Status.CompletionTime.IsZero() {
				t.Fatalf("expected completion time to be zero, got: %v", retrieved.Status.CompletionTime)
			}
			if len(retrieved.Status.Conditions) != 0 {
				t.Fatalf("expected no conditions, got: %v", retrieved.Status.Conditions)
			}

			assertSecureBootCalls(t, tt.action, tt.provider, tt.configMap)

			// Timeout check
			if tt.timeoutErr {
				expired := metav1.NewTime(retrieved.Status.StartTime.Add(-time.Hour))
				retrieved.Status.StartTime = &expired
				if err = cluster.Status().Update(context.Background(), &retrieved); err != nil {
					t.Fatalf("expected nil err, got: %v", err)
				}

				result, err = reconciler.Reconcile(context.Background(), request)
				if err == nil {
					t.Fatalf("expected err, got: %v", err)
				}
				if diff := cmp.Diff(result, ctrl.Result{}); diff != "" {
					t.Fatalf("expected no diff, got: %v", diff)
				}
				return
			}

			// Ensure re-reconciling a task does sends it into a success state.
			result, err = reconciler.Reconcile(context.Background(), request)
			if err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			if diff := cmp.Diff(result, reconcile.Result{}); diff != "" {
				t.Fatalf("expected no diff, got: %v", diff)
			}

			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			if err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			// TODO: g.Expect(retrieved.Status.CompletionTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			if len(retrieved.Status.Conditions) != 1 {
				t.Fatalf("expected 1 condition, got: %v", retrieved.Status.Conditions)
			}
			if retrieved.Status.Conditions[0].Type != bmc.TaskCompleted {
				t.Fatalf("expected condition type to be %s, got: %s", bmc.TaskCompleted, retrieved.Status.Conditions[0].Type)
			}
			if retrieved.Status.Conditions[0].Status != bmc.ConditionTrue {
				t.Fatalf("expected condition status to be %s, got: %s", bmc.ConditionTrue, retrieved.Status.Conditions[0].Status)
			}

			var retrieved2 bmc.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved2)
			if err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			if diff := cmp.Diff(retrieved2, retrieved); diff != "" {
				t.Fatalf("expected no diff, got: %v", diff)
			}
		})
	}
}

// createCertConfigMap returns a ConfigMap, in the namespace of the Tasks createTask builds,
// holding a valid PEM-encoded certificate under the key "cert.pem".
func createCertConfigMap(t *testing.T) *corev1.ConfigMap {
	t.Helper()

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cert",
		},
		Data: map[string]string{
			"cert.pem": createCertificatePEM(t),
		},
	}
}

// createCertificatePEM returns a PEM-encoded self-signed certificate.
func createCertificatePEM(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func createTask(name string, action bmc.Action, secret *corev1.Secret) *bmc.Task {
	return &bmc.Task{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bmc.GroupVersion.String(),
			Kind:       "Task",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: bmc.TaskSpec{
			Task: action,
			Connection: bmc.Connection{
				Host: "host",
				Port: 22,
				AuthSecretRef: corev1.SecretReference{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
				ProviderOptions: &bmc.ProviderOptions{
					Redfish: &bmc.RedfishOptions{
						Port: 443,
					},
				},
			},
		},
	}
}

func createTaskWithRPC(name string, action bmc.Action, secret *corev1.Secret) *bmc.Task {
	task := &bmc.Task{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bmc.GroupVersion.String(),
			Kind:       "Task",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: bmc.TaskSpec{
			Task: action,
			Connection: bmc.Connection{
				Host: "host",
				Port: 22,
				ProviderOptions: &bmc.ProviderOptions{
					RPC: &bmc.RPCOptions{
						ConsumerURL: "http://127.0.0.1:7777",
					},
				},
			},
		},
	}

	if secret != nil {
		task.Spec.Connection.AuthSecretRef = corev1.SecretReference{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}

		task.Spec.Connection.ProviderOptions.RPC.HMAC = &bmc.HMACOpts{
			Secrets: bmc.HMACSecrets{
				"sha256": []corev1.SecretReference{
					{
						Name:      secret.Name,
						Namespace: secret.Namespace,
					},
				},
			},
		}
	}

	return task
}

func TestTaskReconcileSecureBootFailureMarksTaskFailed(t *testing.T) {
	secret := createSecret()
	task := createTask("enable", bmc.Action{SecureBoot: &bmc.SecureBootAction{Enable: toPtr(true)}}, secret)
	cluster := newClientBuilder().WithObjects(task, secret).Build()
	provider := &testProvider{ErrSetSecureBoot: errors.New("boom")}

	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: task.Namespace, Name: task.Name}}
	if _, err := controller.NewTaskReconciler(cluster, newTestClient(provider)).Reconcile(context.Background(), request); err == nil {
		t.Fatal("expected an error, got nil")
	}

	var retrieved bmc.Task
	if err := cluster.Get(context.Background(), request.NamespacedName, &retrieved); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
	if !retrieved.HasCondition(bmc.TaskFailed, bmc.ConditionTrue) {
		t.Fatalf("expected the Task to be Failed, got conditions %+v", retrieved.Status.Conditions)
	}
	if got := retrieved.Status.Conditions[0].Message; !strings.Contains(got, "boom") {
		t.Fatalf("expected the failure message to include the provider error, got %q", got)
	}
}

func TestTaskReconcileCertificateImportBadReference(t *testing.T) {
	validPEM := createCertificatePEM(t)
	certMap := func(namespace string, data map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test-cert"},
			Data:       data,
		}
	}

	tests := map[string]struct {
		configMap *corev1.ConfigMap
	}{
		"configmap missing": {},
		"configmap only in another namespace": {
			configMap: certMap("other", map[string]string{"cert.pem": validPEM}),
		},
		"key missing":      {configMap: certMap("default", map[string]string{"other.pem": validPEM})},
		"empty value":      {configMap: certMap("default", map[string]string{"cert.pem": ""})},
		"not a PEM":        {configMap: certMap("default", map[string]string{"cert.pem": "not a certificate"})},
		"not a PEM cert":   {configMap: certMap("default", map[string]string{"cert.pem": "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"})},
		"invalid cert DER": {configMap: certMap("default", map[string]string{"cert.pem": "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"})},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			action := bmc.Action{SecureBoot: &bmc.SecureBootAction{CertificateImport: &bmc.SecureBootCertificateImportAction{
				Database:                   "db",
				CertificatePEMConfigMapRef: bmc.ConfigMapKeyReference{Name: "test-cert", Key: "cert.pem"},
			}}}
			secret := createSecret()
			task := createTask("import", action, secret)
			objs := []client.Object{task, secret}
			if tt.configMap != nil {
				objs = append(objs, tt.configMap)
			}
			cluster := newClientBuilder().WithObjects(objs...).Build()
			provider := &testProvider{}

			reconciler := controller.NewTaskReconciler(cluster, newTestClient(provider))
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: task.Namespace, Name: task.Name}}

			if _, err := reconciler.Reconcile(context.Background(), request); err == nil {
				t.Fatal("expected an error, got nil")
			}

			// The BMC must not be touched, and the Task must stay retryable rather than Failed.
			if provider.ImportSecureBootCertificateCalledWith != [2]string{} {
				t.Fatalf("expected ImportSecureBootCertificate not to be called, got %v", provider.ImportSecureBootCertificateCalledWith)
			}
			var retrieved bmc.Task
			if err := cluster.Get(context.Background(), request.NamespacedName, &retrieved); err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			if len(retrieved.Status.Conditions) != 0 || !retrieved.Status.StartTime.IsZero() {
				t.Fatalf("expected the Task to be untouched, got status %+v", retrieved.Status)
			}
		})
	}
}

// assertSecureBootCalls checks that the Secure Boot operation set in action reached provider
// with the expected arguments. It does nothing for actions without a Secure Boot operation.
func assertSecureBootCalls(t *testing.T, action bmc.Action, provider *testProvider, configMap *corev1.ConfigMap) {
	t.Helper()

	sb := action.SecureBoot
	if sb == nil {
		return
	}

	if sb.Enable != nil {
		if got, want := provider.SecureBootEnabled, *sb.Enable; got != want {
			t.Fatalf("expected secure boot enabled=%v, got %v", want, got)
		}
	}

	if sb.AllowCustomKeys != nil {
		if got := provider.AllowCustomSecureBootKeysCalledWith; got == nil || *got != *sb.AllowCustomKeys {
			t.Fatalf("expected AllowCustomSecureBootKeys called with %v, got %v", *sb.AllowCustomKeys, got)
		}
	}

	if sb.ResetKeys != nil {
		want := sb.ResetKeys.ResetType
		if got := provider.ResetSecureBootKeysCalledWith; got != want {
			t.Fatalf("expected ResetSecureBootKeys called with %q, got %q", want, got)
		}
	}

	if sb.DatabaseResetKeys != nil {
		want := [2]string{sb.DatabaseResetKeys.Database, sb.DatabaseResetKeys.ResetType}
		if got := provider.ResetSecureBootDatabaseKeysCalledWith; got != want {
			t.Fatalf("expected ResetSecureBootDatabaseKeys called with %v, got %v", want, got)
		}
	}

	if sb.CertificateImport != nil {
		ref := sb.CertificateImport.CertificatePEMConfigMapRef
		want := [2]string{sb.CertificateImport.Database, configMap.Data[ref.Key]}
		if got := provider.ImportSecureBootCertificateCalledWith; got != want {
			t.Fatalf("expected ImportSecureBootCertificate called with %v, got %v", want, got)
		}
	}
}
