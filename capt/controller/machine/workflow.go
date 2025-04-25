package machine

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	tinkv1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	v1beta1 "github.com/tinkerbell/tinkerbell/pkg/api/v1beta1/capt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// errWorkflowFailed is the error returned when the workflow fails.
var errWorkflowFailed = errors.New("workflow failed")

// errISOBootURLRequired is the error returned when the isoURL is required for iso boot mode.
var errISOBootURLRequired = errors.New("iso boot mode requires an isoURL")

func (scope *machineReconcileScope) getWorkflow() (*tinkv1.Workflow, error) {
	namespacedName := types.NamespacedName{
		Name:      scope.tinkerbellMachine.Name,
		Namespace: scope.tinkerbellMachine.Namespace,
	}

	t := &tinkv1.Workflow{}

	err := scope.client.Get(scope.ctx, namespacedName, t)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return t, fmt.Errorf("no workflow exists: %w", err)
		}

		return t, fmt.Errorf("failed to get workflow: %w", err)
	}

	return t, nil
}

func (scope *machineReconcileScope) createWorkflow(hw *tinkv1.Hardware) error {
	c := true
	workflow := &tinkv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.tinkerbellMachine.Name,
			Namespace: scope.tinkerbellMachine.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "TinkerbellMachine",
					Name:       scope.tinkerbellMachine.Name,
					UID:        scope.tinkerbellMachine.GetUID(),
					Controller: &c,
				},
			},
		},
		Spec: tinkv1.WorkflowSpec{
			TemplateRef: scope.tinkerbellMachine.Name,
			HardwareRef: hw.Name,
			HardwareMap: map[string]string{"device_1": hw.Spec.Metadata.Instance.ID},
			BootOptions: tinkv1.BootOptions{
				ToggleAllowNetboot: true,
			},
		},
	}

	// We check the BMCRef so that the implementation behaves similar to how it was when
	// CAPT was creating the BMCJob.
	if hw.Spec.BMCRef != nil {
		switch scope.tinkerbellMachine.Spec.BootOptions.BootMode {
		case v1beta1.BootMode("netboot"):
			workflow.Spec.BootOptions.BootMode = tinkv1.BootMode("netboot")
		case v1beta1.BootMode("iso"):
			if scope.tinkerbellMachine.Spec.BootOptions.ISOURL == "" {
				return errISOBootURLRequired
			}

			u, err := url.Parse(scope.tinkerbellMachine.Spec.BootOptions.ISOURL)
			if err != nil {
				return fmt.Errorf("boot option isoURL is not parse-able: %w", err)
			}

			urlPath, file := path.Split(u.Path)
			u.Path = path.Join(urlPath, strings.Replace(hw.Spec.Metadata.Instance.ID, ":", "-", 5), file)

			workflow.Spec.BootOptions.ISOURL = u.String()
			workflow.Spec.BootOptions.BootMode = tinkv1.BootMode("iso")
		}
	}

	if err := scope.client.Create(scope.ctx, workflow); err != nil {
		return fmt.Errorf("creating workflow: %w", err)
	}

	return nil
}

// removeWorkflow makes sure workflow for TinkerbellMachine has been cleaned up.
func (scope *machineReconcileScope) removeWorkflow() error {
	namespacedName := types.NamespacedName{
		Name:      scope.tinkerbellMachine.Name,
		Namespace: scope.tinkerbellMachine.Namespace,
	}

	workflow := &tinkv1.Workflow{}

	err := scope.client.Get(scope.ctx, namespacedName, workflow)
	if err != nil {
		if apierrors.IsNotFound(err) {
			scope.log.Info("Workflow already removed", "name", namespacedName)

			return nil
		}

		return fmt.Errorf("checking if workflow exists: %w", err)
	}

	scope.log.Info("Removing Workflow", "name", namespacedName)

	if err := scope.client.Delete(scope.ctx, workflow); err != nil {
		return fmt.Errorf("ensuring workflow has been removed: %w", err)
	}

	return nil
}
