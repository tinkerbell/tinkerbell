package tinkerbell

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkflowMethods(t *testing.T) {
	cases := []struct {
		name string
		wf   *Workflow
		want taskInfo
	}{
		{
			"Empty wflow",
			&Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
			},
			taskInfo{},
		},
		{
			"invalid workflow",
			&Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: WorkflowSpec{},
				Status: WorkflowStatus{
					State:         WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []Task{
						{
							Name: "empty task",
							// WorkerAddr: "", // intentionally not set
							Actions: []Action{
								{
									Name:   "empty action",
									Status: WorkflowStateFailed,
								},
							},
						},
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStateSuccess,

									Seconds: 20,
								},
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStateRunning,
								},
							},
						},
					},
				},
			},
			taskInfo{
				TotalNumberOfActions: 3,
				CurrentTaskIndex:     0,
				CurrentTask:          "empty task",
				CurrentWorker:        "",
				CurrentAction:        "empty action",
				CurrentActionState:   WorkflowStateFailed,
				CurrentActionIndex:   0,
			},
		},
		{
			"Running workflow",
			&Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: WorkflowSpec{},
				Status: WorkflowStatus{
					State:         WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []Task{
						{
							Name:       "bmc-manage",
							WorkerAddr: "pbnj",
							Actions: []Action{
								{
									Name:    "configure-pxe",
									Image:   "quay.io/tinkerbell-actions/pbnj:v1.0.0",
									Timeout: 20,
									Status:  WorkflowStateSuccess,

									Seconds: 15,
								},
							},
						},
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStateSuccess,

									Seconds: 20,
								},
								{
									Name:    "write-file",
									Image:   "quay.io/tinkerbell-actions/writefile:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStateRunning,
								},
							},
						},
					},
				},
			},
			taskInfo{
				TotalNumberOfActions: 3,
				CurrentTaskIndex:     1,
				CurrentTask:          "os-installation",
				CurrentWorker:        "3c:ec:ef:4c:4f:54",
				CurrentAction:        "write-file",
				CurrentActionState:   WorkflowStateRunning,
				CurrentActionIndex:   2,
			},
		},
		{
			"Pending workflow",
			&Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: WorkflowSpec{},
				Status: WorkflowStatus{
					State:         WorkflowStatePending,
					GlobalTimeout: 600,
					Tasks: []Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStatePending,
								},
								{
									Name:    "write-file",
									Image:   "quay.io/tinkerbell-actions/writefile:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			taskInfo{
				TotalNumberOfActions: 2,
				CurrentTaskIndex:     0,
				CurrentTask:          "os-installation",
				CurrentWorker:        "3c:ec:ef:4c:4f:54",
				CurrentAction:        "stream-debian-image",
				CurrentActionState:   WorkflowStatePending,
				CurrentActionIndex:   0,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getTaskActionInfo(tc.wf)
			if got != tc.want {
				t.Errorf("Got \n\t%#v\nwanted:\n\t%#v", got, tc.want)
			}
		})
	}
}

func TestSetCondition(t *testing.T) {
	tests := map[string]struct {
		ExistingConditions []WorkflowCondition
		WantConditions     []WorkflowCondition
		Condition          WorkflowCondition
	}{
		"update existing condition": {
			ExistingConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			Condition: WorkflowCondition{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
		},
		"append new condition": {
			ExistingConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
			},
			Condition: WorkflowCondition{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := &WorkflowStatus{
				Conditions: tt.ExistingConditions,
			}
			w.SetCondition(tt.Condition)
			if !cmp.Equal(tt.WantConditions, w.Conditions) {
				t.Errorf("SetCondition() mismatch (-want +got):\n%s", cmp.Diff(tt.WantConditions, w.Conditions))
			}
		})
	}
}
