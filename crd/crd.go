package crd

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

//go:embed bases/tinkerbell.org_hardware.yaml
var HardwareCRD []byte

//go:embed bases/tinkerbell.org_templates.yaml
var TemplateCRD []byte

//go:embed bases/tinkerbell.org_workflows.yaml
var WorkflowCRD []byte

//go:embed bases/bmc.tinkerbell.org_jobs.yaml
var JobCRD []byte

//go:embed bases/bmc.tinkerbell.org_machines.yaml
var MachineCRD []byte

//go:embed bases/bmc.tinkerbell.org_tasks.yaml
var TaskCRD []byte

func crdNames() []string {
	return []string{
		"hardware.tinkerbell.org",
		"templates.tinkerbell.org",
		"workflows.tinkerbell.org",
		"machines.bmc.tinkerbell.org",
		"jobs.bmc.tinkerbell.org",
		"tasks.bmc.tinkerbell.org",
	}
}

// Migrate applies the CRDs to the cluster.
func Migrate(ctx context.Context, log logr.Logger, config *rest.Config) error {
	// TODO: should we check for differences in the CRDs? Should we check for the presence of the CRDs?
	// This function should eventually grow to handle upgrades.

	// Wait for the API server to be healthy
	if err := waitForAPIServer(ctx, log, config, 20*time.Second, 5*time.Second); err != nil {
		return fmt.Errorf("failed to wait for API server health: %w", err)
	}

	apiExtClient, err := apiextensionsv1client.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	for _, raw := range [][]byte{HardwareCRD, TemplateCRD, WorkflowCRD, MachineCRD, JobCRD, TaskCRD} {
		obj := &unstructured.Unstructured{}
		if _, _, err := decoder.Decode(raw, nil, obj); err != nil {
			return fmt.Errorf("failed to decode YAML: %w", err)
		}
		crdef := &apiextensionsv1.CustomResourceDefinition{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, crdef); err != nil {
			return fmt.Errorf("failed to convert unstructured to CRD: %w", err)
		}

		if _, err := apiExtClient.CustomResourceDefinitions().Create(ctx, crdef, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CRD: %w", err)
		}
	}

	// Get the CRDs from the cluster to verify they are available and usable.
	for _, name := range crdNames() {
		if err := retry.Do(func() error {
			log.V(1).Info("Checking for CRD", "name", name)
			if _, err := apiExtClient.CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("failed to get CRD %s: %w", name, err)
			}
			return nil
		}, retry.Attempts(5), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
			return fmt.Errorf("failed to waiting for CRD %s: %w", name, err)
		}
	}

	log.Info("CRDs created")
	return nil
}

// waitForAPIServer waits for the Kubernetes API server to become healthy.
func waitForAPIServer(ctx context.Context, log logr.Logger, config *rest.Config, maxWaitTime time.Duration, pollInterval time.Duration) error {
	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Calculate deadline
	deadline := time.Now().Add(maxWaitTime)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Check health
			resp, err := clientset.Discovery().RESTClient().Get().AbsPath("/livez").Do(ctx).Raw()

			if err == nil && len(resp) > 0 {
				log.Info("API server is healthy")
				return nil // API server is healthy
			}

			// Log error and continue waiting
			log.Info("API server not healthy yet, will retry", "retryInterval", fmt.Sprintf("%v", pollInterval), "reason", err)

			// Sleep before next check
			time.Sleep(pollInterval)
		}
	}

	return fmt.Errorf("timed out waiting for API server health after %v", maxWaitTime)
}
