package crd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/client/applyconfiguration/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
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

// Tinkerbell is the struct that holds the raw custom resource definitions
// and a CRD client for operations.
type Tinkerbell struct {
	CRDs   map[string][]byte
	Client clientset.Interface
}

const (
	// HardwareCRDName is the name of the Hardware CRD.
	HardwareCRDName = "hardware.tinkerbell.org"
	// TemplateCRDName is the name of the Template CRD.
	TemplateCRDName = "templates.tinkerbell.org"
	// WorkflowCRDName is the name of the Workflow CRD.
	WorkflowCRDName = "workflows.tinkerbell.org"
	// JobCRDName is the name of the Job CRD.
	JobCRDName = "jobs.bmc.tinkerbell.org"
	// MachineCRDName is the name of the Machine CRD.
	MachineCRDName = "machines.bmc.tinkerbell.org"
	// TaskCRDName is the name of the Task CRD.
	TaskCRDName = "tasks.bmc.tinkerbell.org"
)

// TinkerbellDefaults contains all the Tinkerbell CRDs.
var TinkerbellDefaults = map[string][]byte{
	HardwareCRDName: HardwareCRD,
	TemplateCRDName: TemplateCRD,
	WorkflowCRDName: WorkflowCRD,
	JobCRDName:      JobCRD,
	MachineCRDName:  MachineCRD,
	TaskCRDName:     TaskCRD,
}

// ConfigOption is a function that sets a configuration option.
type ConfigOption func(*Tinkerbell)

// WithClient sets the client in the Mapping.
func WithClient(config clientset.Interface) ConfigOption {
	return func(m *Tinkerbell) {
		m.Client = config
	}
}

func WithRestConfig(config *rest.Config) ConfigOption {
	return func(m *Tinkerbell) {
		client, err := clientset.NewForConfig(config)
		if err != nil {
			panic(err)
		}
		m.Client = client
	}
}

// NewTinkerbell returns a struct with a CRD client and the CRDs.
// If no CRDs are provided, it will use the default (TinkerbellDefaults) CRDs.
func NewTinkerbell(opts ...ConfigOption) Tinkerbell {
	defaultMapper := Tinkerbell{
		CRDs: TinkerbellDefaults,
	}
	for _, opt := range opts {
		opt(&defaultMapper)
	}
	return defaultMapper
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

		if _, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, obj.GetName(), metav1.GetOptions{}); err == nil {
			continue
		}
		// Try apply, if that fails, try create. Apply only works if the CRD already exists.
		if errApply := t.apply(ctx, obj); errApply != nil {
			if errCreate := t.create(ctx, obj); errCreate != nil {
				return errors.Join(errApply, errCreate)
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
			if establishedCond.Status != apiv1.ConditionTrue && namesAcceptedCond.Status != apiv1.ConditionTrue {
				return fmt.Errorf("CRD %s is not ready: established: %v, namesAccepted: %v", name, establishedCond.Status, namesAcceptedCond.Status)
			}
			return nil
		}, retry.Attempts(5), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
			return fmt.Errorf("failed to waiting for CRD %s: %w", name, err)
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

	if _, err := t.Client.ApiextensionsV1().CustomResourceDefinitions().Apply(ctx, crdef, metav1.ApplyOptions{FieldManager: "Tinkerbell CLI"}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to apply CRD: %w", err)
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
