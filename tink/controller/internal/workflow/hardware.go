package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// valueToPointer converts a value of any type to a pointer of that type.
func valueToPointer[T any](v T) *T {
	return &v
}

func defaultBackoff() func() time.Duration {
	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.MaxInterval = time.Second
	return backoffPolicy.NextBackOff
}

type backoffConfig struct {
	maxRetries int
	duration   func() time.Duration
}

type backoffOpts func(*backoffConfig)

type allowPXEJSONPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// setAllowPXE sets the allowPXE field on the hardware network interfaces.
// If hardware is nil then it will be retrieved using the client.
// The hardware object will be updated in the cluster.
func setAllowPXE(ctx context.Context, cc client.Client, w *v1alpha1.Workflow, h *v1alpha1.Hardware, allowPXE bool, opts ...backoffOpts) error {
	if h == nil && w == nil {
		return fmt.Errorf("both workflow and hardware cannot be nil")
	}
	bc := &backoffConfig{
		maxRetries: 4,
		duration:   defaultBackoff(),
	}
	for _, opt := range opts {
		opt(bc)
	}
	var key client.ObjectKey
	if h != nil {
		key = client.ObjectKeyFromObject(h)
	} else {
		key = client.ObjectKey{Name: w.Spec.HardwareRef, Namespace: w.Namespace}
	}
	for attempt := 1; attempt <= bc.maxRetries; attempt++ {
		if h == nil || h.ResourceVersion == "" {
			h = &v1alpha1.Hardware{}
			if err := cc.Get(ctx, key, h); err != nil {
				return fmt.Errorf("hardware not found: name=%v; namespace=%v, error: %w", key.Name, key.Namespace, err)
			}
		}

		operations := []allowPXEJSONPatchOperation{{
			Op:    "test",
			Path:  "/metadata/resourceVersion",
			Value: h.ResourceVersion,
		}}
		for idx := range h.Spec.Interfaces {
			path := fmt.Sprintf("/spec/interfaces/%d/netboot", idx)
			if h.Spec.Interfaces[idx].Netboot == nil {
				operations = append(operations, allowPXEJSONPatchOperation{
					Op:    "add",
					Path:  path,
					Value: map[string]any{"allowPXE": allowPXE},
				})
				continue
			}
			operations = append(operations, allowPXEJSONPatchOperation{
				Op:    "add",
				Path:  path + "/allowPXE",
				Value: allowPXE,
			})
		}
		patchData, err := json.Marshal(operations)
		if err != nil {
			return fmt.Errorf("failed to marshal allowPXE patch for Hardware %s/%s: %w", h.Namespace, h.Name, err)
		}
		if err := cc.Patch(ctx, h, client.RawPatch(types.JSONPatchType, patchData)); err != nil {
			if isAllowPXEConflict(err) {
				if attempt >= bc.maxRetries {
					return fmt.Errorf("error updating allow pxe after %d retries: %w", attempt, err)
				}
				h = nil // reset h to nil to retry fetching the hardware
				// This is a conflict error, which means the hardware object was updated by another process
				// We will retry fetching the hardware object and updating it again.
				time.Sleep(bc.duration())
				continue
			}
			return fmt.Errorf("error updating allow pxe: %w", err)
		}
		return nil
	}

	return nil
}

// hardwareFrom retrieves the in cluster hardware object defined in the given workflow.
func hardwareFrom(ctx context.Context, cc client.Client, w *v1alpha1.Workflow) (*v1alpha1.Hardware, error) {
	if w == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	h := &v1alpha1.Hardware{}
	if err := cc.Get(ctx, client.ObjectKey{Name: w.Spec.HardwareRef, Namespace: w.Namespace}, h); err != nil {
		return nil, fmt.Errorf("hardware not found: name=%v; namespace=%v, error: %w", w.Spec.HardwareRef, w.Namespace, err)
	}

	return h, nil
}

// toggleHardware toggles the allowPXE field on the hardware network interfaces.
// It is idempotent and uses the Workflow.Status.BootOptionsStatus.AllowNetboot fields for idempotent checks.
// This function will update the Workflow status.
func (s *state) toggleHardware(ctx context.Context, allowPXE bool) error {
	// 1. check if we've already set the allowPXE field to the desired value
	// 2. if not, set the allowPXE field to the desired value
	// 3. return a WorkflowCondition with the result of the operation

	hw, err := hardwareFrom(ctx, s.client, s.workflow)
	if err != nil {
		s.workflow.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.ToggleAllowNetbootTrue,
			Status:  metav1.ConditionFalse,
			Reason:  reasonError,
			Message: fmt.Sprintf("error getting hardware: %v", err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})

		return err
	}

	if allowPXE {
		if s.workflow.Status.BootOptions.AllowNetboot.ToggledTrue {
			return nil
		}
		if err := setAllowPXE(ctx, s.client, s.workflow, hw, allowPXE); err != nil {
			s.workflow.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
				Type:    v1alpha1.ToggleAllowNetbootTrue,
				Status:  metav1.ConditionFalse,
				Reason:  reasonError,
				Message: fmt.Sprintf("error setting allowPXE to %v: %v", allowPXE, err),
				Time:    &metav1.Time{Time: metav1.Now().UTC()},
			})
			return err
		}
		s.workflow.Status.BootOptions.AllowNetboot.ToggledTrue = true
		s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.ToggleAllowNetbootTrue,
			Status:  metav1.ConditionTrue,
			Reason:  "Complete",
			Message: fmt.Sprintf("set allowPXE to %v", allowPXE),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return nil
	}

	if s.workflow.Status.BootOptions.AllowNetboot.ToggledFalse {
		return nil
	}
	if err := setAllowPXE(ctx, s.client, s.workflow, hw, allowPXE); err != nil {
		s.workflow.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.ToggleAllowNetbootFalse,
			Status:  metav1.ConditionFalse,
			Reason:  reasonError,
			Message: fmt.Sprintf("error setting allowPXE to %v: %v", allowPXE, err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return err
	}
	s.workflow.Status.BootOptions.AllowNetboot.ToggledFalse = true
	s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
		Type:    v1alpha1.ToggleAllowNetbootFalse,
		Status:  metav1.ConditionTrue,
		Reason:  "Complete",
		Message: fmt.Sprintf("set allowPXE to %v", allowPXE),
		Time:    &metav1.Time{Time: metav1.Now().UTC()},
	})
	return nil
}

func isAllowPXEConflict(err error) bool {
	if apierrors.IsConflict(err) {
		return true
	}
	if !apierrors.IsInvalid(err) && !apierrors.IsBadRequest(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "test failed") && strings.Contains(message, "resourceversion")
}
