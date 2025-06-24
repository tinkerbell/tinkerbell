package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPostActions(t *testing.T) {
	tests := map[string]struct {
		wantResult   reconcile.Result
		wantError    bool
		hardware     *v1alpha1.Hardware
		wantHardware *v1alpha1.Hardware
		workflow     *v1alpha1.Workflow
		wantWorkflow *v1alpha1.Workflow
		job          *bmc.Job
	}{
		"nothing to do": {
			wantResult:   reconcile.Result{},
			hardware:     &v1alpha1.Hardware{},
			wantHardware: &v1alpha1.Hardware{},
			workflow:     &v1alpha1.Workflow{},
			wantWorkflow: &v1alpha1.Workflow{},
		},
		"toggle allowPXE false": {
			wantResult: reconcile.Result{},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			wantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: v1alpha1.BootOptions{
						ToggleAllowNetboot: true,
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						AllowNetboot: v1alpha1.AllowNetbootStatus{
							ToggledFalse: false,
						},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateSuccess,
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						AllowNetboot: v1alpha1.AllowNetbootStatus{
							ToggledFalse: true,
						},
					},
					Conditions: []v1alpha1.WorkflowCondition{
						{
							Type:    v1alpha1.ToggleAllowNetbootFalse,
							Status:  metav1.ConditionTrue,
							Reason:  "Complete",
							Message: "set allowPXE to false",
						},
					},
				},
			},
		},
		"iso eject": {
			wantResult: reconcile.Result{
				Requeue: true,
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			wantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeISO,
						ISOURL:   "http://example.com/iso.iso",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameISOEject): {ExistingJobDeleted: true},
						},
					},
				},
			},
		},
		"iso eject no url": {
			wantResult: reconcile.Result{},
			wantError:  true,
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			wantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeISO,
						// Missing ISOURL
					},
				},
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateFailed,
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{},
					},
				},
			},
		},
		"iso eject complete": {
			wantResult: reconcile.Result{},
			hardware:   &v1alpha1.Hardware{},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeISO,
						ISOURL:   "http://example.com/iso.iso",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameISOEject): {
								ExistingJobDeleted: true,
								UID:                "test-uid",
								Complete:           true,
							},
						},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateSuccess,
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameISOEject): {
								ExistingJobDeleted: true,
								UID:                "test-uid",
								Complete:           true,
							},
						},
					},
				},
			},
		},
		"customboot post actions": {
			wantResult: reconcile.Result{
				Requeue: true,
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			wantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeCustomboot,
						CustombootConfig: v1alpha1.CustombootConfig{
							PostActions: []bmc.Action{
								{
									PowerAction: valueToPointer(bmc.PowerHardOff),
								},
							},
						},
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameCustombootPost): {ExistingJobDeleted: true},
						},
					},
				},
			},
		},
		"customboot post actions complete": {
			wantResult: reconcile.Result{},
			hardware:   &v1alpha1.Hardware{},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeCustomboot,
						CustombootConfig: v1alpha1.CustombootConfig{
							PostActions: []bmc.Action{
								{
									PowerAction: valueToPointer(bmc.PowerHardOff),
								},
							},
						},
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameCustombootPost): {
								ExistingJobDeleted: true,
								UID:                "test-uid",
								Complete:           true,
							},
						},
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateSuccess,
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameCustombootPost): {
								ExistingJobDeleted: true,
								UID:                "test-uid",
								Complete:           true,
							},
						},
					},
				},
			},
		},
		"netboot mode": {
			wantResult: reconcile.Result{},
			hardware:   &v1alpha1.Hardware{},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					BootOptions: v1alpha1.BootOptions{
						BootMode: v1alpha1.BootModeNetboot,
					},
				},
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
				},
			},
			wantWorkflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateSuccess,
					CurrentState: &v1alpha1.CurrentState{
						State: v1alpha1.WorkflowStateSuccess,
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			api.AddToSchemeBMC(scheme)
			api.AddToSchemeTinkerbell(scheme)
			ro := []runtime.Object{}
			if tc.hardware != nil {
				ro = append(ro, tc.hardware)
			}
			if tc.workflow != nil {
				ro = append(ro, tc.workflow)
			}
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ro...)
			if tc.job != nil {
				clientBuilder.WithRuntimeObjects(tc.job)
			}
			s := &state{
				workflow: tc.workflow,
				client:   clientBuilder.Build(),
			}
			ctx := context.Background()
			ctx = journal.New(ctx)
			result, err := s.postActions(ctx)
			if (err != nil) != tc.wantError {
				t.Errorf("expected error: %v, got: %v", tc.wantError, err)
			}
			if diff := cmp.Diff(result, tc.wantResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
				t.Logf("journal: %v", journal.Journal(ctx))
			}

			// get the Hardware object in cluster if it exists
			if tc.hardware != nil && tc.hardware.Name != "" {
				gotHardware := &v1alpha1.Hardware{}
				if err := s.client.Get(ctx, types.NamespacedName{Name: tc.hardware.Name, Namespace: tc.hardware.Namespace}, gotHardware); err != nil {
					t.Fatalf("error getting hardware: %v", err)
				}
				if diff := cmp.Diff(gotHardware.Spec, tc.wantHardware.Spec); diff != "" {
					t.Errorf("unexpected hardware (-want +got):\n%s", diff)
					for _, entry := range journal.Journal(ctx) {
						t.Logf("journal: %+v", entry)
					}
				}
			}

			// Compare workflow status, ignoring time fields
			if diff := cmp.Diff(tc.workflow.Status, tc.wantWorkflow.Status, cmpopts.IgnoreFields(v1alpha1.WorkflowCondition{}, "Time")); diff != "" {
				t.Errorf("unexpected workflow status (-want +got):\n%s", diff)
				for _, entry := range journal.Journal(ctx) {
					t.Logf("journal: %+v", entry)
				}
			}
		})
	}
}
