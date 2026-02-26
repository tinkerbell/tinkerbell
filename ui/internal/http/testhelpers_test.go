package webhttp

import (
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	bmcv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testLog = logr.Discard()

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestScheme creates a scheme with all required types registered.
func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = api.AddToSchemeTinkerbell(scheme)
	_ = api.AddToSchemeBMC(scheme)
	return scheme
}

// newFakeKubeClient creates a KubeClient with a fake underlying client.
func newFakeKubeClient(objs ...runtime.Object) *KubeClient {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
	return &KubeClient{Client: fakeClient}
}

// setupTestContext creates a gin context with a fake kube client injected.
func setupTestContext(path string, kubeClient *KubeClient) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Set("kubeClient", kubeClient)
	return c, w
}

// Test data factories.

func newTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newTestHardware(name, namespace, mac, ip string) *tinkv1alpha1.Hardware {
	hw := &tinkv1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: tinkv1alpha1.HardwareSpec{},
	}
	if mac != "" || ip != "" {
		hw.Spec.Interfaces = []tinkv1alpha1.Interface{
			{
				DHCP: &tinkv1alpha1.DHCP{
					MAC: mac,
				},
			},
		}
		if ip != "" {
			hw.Spec.Interfaces[0].DHCP.IP = &tinkv1alpha1.IP{Address: ip}
		}
	}
	return hw
}

func newTestWorkflow(name, templateRef string, state tinkv1alpha1.WorkflowState) *tinkv1alpha1.Workflow {
	return &tinkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: tinkv1alpha1.WorkflowSpec{
			TemplateRef: templateRef,
		},
		Status: tinkv1alpha1.WorkflowStatus{
			State: state,
		},
	}
}

func newTestTemplate(name string, data *string) *tinkv1alpha1.Template {
	return &tinkv1alpha1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: tinkv1alpha1.TemplateSpec{
			Data: data,
		},
	}
}

func newTestBMCMachine(name, namespace, host string, power bmcv1alpha1.PowerState) *bmcv1alpha1.Machine {
	return &bmcv1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: bmcv1alpha1.MachineSpec{
			Connection: bmcv1alpha1.Connection{
				Host: host,
			},
		},
		Status: bmcv1alpha1.MachineStatus{
			Power: power,
		},
	}
}

func newTestBMCJob(name, namespace, machine string, conditions []bmcv1alpha1.JobCondition) *bmcv1alpha1.Job {
	return &bmcv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: bmcv1alpha1.JobSpec{
			MachineRef: bmcv1alpha1.MachineRef{
				Name:      machine,
				Namespace: namespace,
			},
		},
		Status: bmcv1alpha1.JobStatus{
			Conditions: conditions,
		},
	}
}

func newTestBMCTask(name, namespace string, power bmcv1alpha1.PowerAction) *bmcv1alpha1.Task { //nolint:unparam // not a problem that namespace always receives "default".
	return &bmcv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: bmcv1alpha1.TaskSpec{
			Task: bmcv1alpha1.Action{
				PowerAction: &power,
			},
		},
	}
}

func newTestWorkflowRuleSet(name, namespace, templateRef string, rules []string) *tinkv1alpha1.WorkflowRuleSet {
	return &tinkv1alpha1.WorkflowRuleSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: tinkv1alpha1.WorkflowRuleSetSpec{
			Rules: rules,
			Workflow: tinkv1alpha1.WorkflowRuleSetWorkflow{
				Template: tinkv1alpha1.TemplateConfig{
					Ref: templateRef,
				},
			},
		},
	}
}

// Helper function for string containment check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
