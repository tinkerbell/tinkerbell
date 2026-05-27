package crd

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/client/applyconfiguration/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/rest"
)

//go:embed bases
var crdFS embed.FS

func mustReadCRD(path string) []byte {
	data, err := crdFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("reading embedded CRD %s: %v", path, err))
	}
	return data
}

// Tinkerbell is the struct that holds the raw custom resource definitions
// and a CRD client for operations.
type Tinkerbell struct {
	CRDs       map[string][]byte
	Client     clientset.Interface
	Logger     logr.Logger
	restConfig *rest.Config
}

const (
	// GroupTinkerbell is the API group for core Tinkerbell resources.
	GroupTinkerbell = "tinkerbell.org"
	// GroupBMC is the API group for BMC resources.
	GroupBMC = "bmc.tinkerbell.org"
)

// AvailableVersions lists all supported CRD API versions.
var AvailableVersions = []string{"v1alpha1", "v1alpha2"}

// TinkerbellDefaults contains all the v1alpha1 Tinkerbell CRDs.
var TinkerbellDefaults = map[string][]byte{
	"hardware.tinkerbell.org":         mustReadCRD("bases/v1alpha1/tinkerbell.org_hardware.yaml"),
	"templates.tinkerbell.org":        mustReadCRD("bases/v1alpha1/tinkerbell.org_templates.yaml"),
	"workflows.tinkerbell.org":        mustReadCRD("bases/v1alpha1/tinkerbell.org_workflows.yaml"),
	"workflowrulesets.tinkerbell.org": mustReadCRD("bases/v1alpha1/tinkerbell.org_workflowrulesets.yaml"),
	"jobs.bmc.tinkerbell.org":         mustReadCRD("bases/v1alpha1/bmc.tinkerbell.org_jobs.yaml"),
	"machines.bmc.tinkerbell.org":     mustReadCRD("bases/v1alpha1/bmc.tinkerbell.org_machines.yaml"),
	"tasks.bmc.tinkerbell.org":        mustReadCRD("bases/v1alpha1/bmc.tinkerbell.org_tasks.yaml"),
}

// TinkerbellV1Alpha2 contains all the v1alpha2 Tinkerbell CRDs.
var TinkerbellV1Alpha2 = map[string][]byte{
	"hardware.tinkerbell.org":  mustReadCRD("bases/v1alpha2/tinkerbell.org_hardware.yaml"),
	"tasks.tinkerbell.org":     mustReadCRD("bases/v1alpha2/tinkerbell.org_tasks.yaml"),
	"bmcs.tinkerbell.org":      mustReadCRD("bases/v1alpha2/tinkerbell.org_bmcs.yaml"),
	"workflows.tinkerbell.org": mustReadCRD("bases/v1alpha2/tinkerbell.org_workflows.yaml"),
	"policies.tinkerbell.org":  mustReadCRD("bases/v1alpha2/tinkerbell.org_policies.yaml"),
	"jobs.bmc.tinkerbell.org":  mustReadCRD("bases/v1alpha2/bmc.tinkerbell.org_jobs.yaml"),
}

// CRDsByVersion maps API version strings to their CRD source maps.
var CRDsByVersion = map[string]map[string][]byte{
	"v1alpha1": TinkerbellDefaults,
	"v1alpha2": TinkerbellV1Alpha2,
}

// ConfigOption is a function that sets a configuration option.
type ConfigOption func(*Tinkerbell)

func WithRestConfig(config *rest.Config) ConfigOption {
	return func(t *Tinkerbell) {
		t.restConfig = config
	}
}

// WithLogger sets a structured logger for Kubernetes API server warnings.
func WithLogger(logger logr.Logger) ConfigOption {
	return func(t *Tinkerbell) {
		t.Logger = logger
	}
}

// logrWarningHandler adapts a logr.Logger to the rest.WarningHandler interface.
type logrWarningHandler struct {
	logger logr.Logger
}

func (h logrWarningHandler) HandleWarningHeader(code int, agent string, text string) {
	h.logger.Info("Kubernetes API warning", "code", code, "agent", agent, "text", text)
}

// NewTinkerbell returns a struct with a CRD client and the CRDs.
// If no CRDs are provided, it will use the default (TinkerbellDefaults) CRDs.
func NewTinkerbell(opts ...ConfigOption) (Tinkerbell, error) {
	tbell := Tinkerbell{
		CRDs: TinkerbellDefaults,
	}
	for _, opt := range opts {
		opt(&tbell)
	}

	if tbell.restConfig != nil {
		cfg := rest.CopyConfig(tbell.restConfig)
		if tbell.Logger.GetSink() != nil {
			cfg.WarningHandler = logrWarningHandler{logger: tbell.Logger}
		}
		client, err := clientset.NewForConfig(cfg)
		if err != nil {
			return Tinkerbell{}, fmt.Errorf("failed to create CRD client: %w", err)
		}
		tbell.Client = client
	}

	if tbell.Client == nil {
		return Tinkerbell{}, fmt.Errorf("no Kubernetes client configured: provide a rest.Config (e.g. WithRestConfig) or set Client directly via a ConfigOption")
	}

	return tbell, nil
}

func (t Tinkerbell) MigrateAndReady(ctx context.Context) error {
	if err := t.Migrate(ctx); err != nil {
		return err
	}

	return t.Ready(ctx)
}

// Migrate applies the CRDs to the cluster.
func (t Tinkerbell) Migrate(ctx context.Context) error {
	// TODO: should we check for differences in the CRDs? Should we check for the presence of the CRDs?
	// This function should eventually grow to handle upgrades.
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	for _, raw := range t.CRDs {
		obj := &unstructured.Unstructured{}
		if _, _, err := decoder.Decode(raw, nil, obj); err != nil {
			return fmt.Errorf("failed to decode YAML: %w", err)
		}

		// Try apply, if that fails, try create. Apply only works if the CRD already exists.
		if errApply := t.apply(ctx, obj); errApply != nil {
			if errUpdate := t.update(ctx, obj); errUpdate != nil {
				if errCreate := t.create(ctx, obj); errCreate != nil {
					return errors.Join(errApply, errUpdate, errCreate)
				}
			}
		}
	}

	return nil
}

// Ready checks if the CRDs exist in the cluster and are established.
func (t Tinkerbell) Ready(ctx context.Context) error {
	// Get the CRDs from the cluster to verify they are available and usable.
	for name := range t.CRDs {
		if err := retry.Do(func() error {
			crd, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get CRD %s: %w", name, err)
			}

			establishedCond := getCondition(crd, apiv1.Established)
			namesAcceptedCond := getCondition(crd, apiv1.NamesAccepted)
			if establishedCond == nil || establishedCond.Status != apiv1.ConditionTrue {
				return fmt.Errorf("CRD %s is not established yet", name)
			}
			if namesAcceptedCond == nil || namesAcceptedCond.Status != apiv1.ConditionTrue {
				return fmt.Errorf("CRD %s names are not accepted yet", name)
			}
			return nil
		}, retry.Attempts(5), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
			return fmt.Errorf("failed waiting for CRD %s to be ready: %w", name, err)
		}
	}

	return nil
}

func (t Tinkerbell) create(ctx context.Context, obj *unstructured.Unstructured) error {
	var crdef apiv1.CustomResourceDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crdef); err != nil {
		return fmt.Errorf("failed to convert unstructured to CRD: %w", err)
	}
	if _, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crdef, metav1.CreateOptions{FieldManager: "Tinkerbell CLI"}); err != nil {
		return fmt.Errorf("failed to create CRD: %w", err)
	}

	return nil
}

func (t Tinkerbell) apply(ctx context.Context, obj *unstructured.Unstructured) error {
	crdef := &v1.CustomResourceDefinitionApplyConfiguration{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, crdef); err != nil {
		return fmt.Errorf("failed to convert unstructured to CRD: %w", err)
	}

	if _, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Apply(ctx, crdef, metav1.ApplyOptions{FieldManager: "Tinkerbell CLI"}); err != nil {
		return fmt.Errorf("failed to apply CRD: %w", err)
	}

	return nil
}

func (t Tinkerbell) update(ctx context.Context, obj *unstructured.Unstructured) error {
	var crdef apiv1.CustomResourceDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crdef); err != nil {
		return fmt.Errorf("failed to convert unstructured to CRD: %w", err)
	}
	// Get the existing CRD to update it.
	existingCRD, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get existing CRD %s: %w", crdef.Name, err)
	}
	// Update the existing CRD with the new spec.
	crdef.ResourceVersion = existingCRD.ResourceVersion
	crdef.UID = existingCRD.UID
	crdef.CreationTimestamp = existingCRD.CreationTimestamp
	if _, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crdef, metav1.UpdateOptions{FieldManager: "Tinkerbell CLI"}); err != nil {
		return fmt.Errorf("failed to update CRD: %w", err)
	}

	return nil
}

// getCondition returns a condition from a list of conditions if it exists.
func getCondition(crd *apiv1.CustomResourceDefinition, conditionType apiv1.CustomResourceDefinitionConditionType) *apiv1.CustomResourceDefinitionCondition {
	for _, cond := range crd.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}
