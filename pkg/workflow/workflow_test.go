package workflow

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStartTime(t *testing.T) {
	cases := []struct {
		name  string
		input *v1alpha1.Workflow
		want  *metav1.Time
	}{
		{
			"Empty wflow",
			&v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
			},
			nil,
		},
		{
			"Running workflow",
			&v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: v1alpha1.WorkflowStateSuccess,

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
									Status: v1alpha1.WorkflowStateRunning,
								},
							},
						},
					},
				},
			},
			nil,
		},
		{
			"pending without a start time",
			&v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStatePending,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status:    v1alpha1.WorkflowStatePending,
									StartedAt: nil,
								},
							},
						},
					},
				},
			},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetStartTime(tc.input)
			if got == nil && tc.want == nil {
				return
			}
			if !got.Time.Equal(tc.want.Time) {
				t.Errorf("Got time %s, wanted %s", got.Format(time.RFC1123), tc.want.Time.Format(time.RFC1123))
			}
		})
	}
}

func TestWorkflowMethods(t *testing.T) {
	cases := []struct {
		name string
		wf   *v1alpha1.Workflow
		want taskInfo
	}{
		{
			"Empty wflow",
			&v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
			},
			taskInfo{},
		},
		{
			"invalid workflow",
			&v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name: "empty task",
							// WorkerAddr: "", // intentionally not set
							Actions: []v1alpha1.Action{
								{
									Name:   "empty action",
									Status: v1alpha1.WorkflowStateFailed,
								},
							},
						},
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: v1alpha1.WorkflowStateSuccess,

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
									Status: v1alpha1.WorkflowStateRunning,
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
				CurrentActionState:   v1alpha1.WorkflowStateFailed,
				CurrentActionIndex:   0,
			},
		},
		{
			"Running workflow",
			&v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "bmc-manage",
							WorkerAddr: "pbnj",
							Actions: []v1alpha1.Action{
								{
									Name:    "configure-pxe",
									Image:   "quay.io/tinkerbell-actions/pbnj:v1.0.0",
									Timeout: 20,
									Status:  v1alpha1.WorkflowStateSuccess,

									Seconds: 15,
								},
							},
						},
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: v1alpha1.WorkflowStateSuccess,

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
									Status: v1alpha1.WorkflowStateRunning,
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
				CurrentActionState:   v1alpha1.WorkflowStateRunning,
				CurrentActionIndex:   2,
			},
		},
		{
			"Pending workflow",
			&v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStatePending,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: v1alpha1.WorkflowStatePending,
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
									Status: v1alpha1.WorkflowStatePending,
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
				CurrentActionState:   v1alpha1.WorkflowStatePending,
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
		ExistingConditions []v1alpha1.WorkflowCondition
		WantConditions     []v1alpha1.WorkflowCondition
		Condition          v1alpha1.WorkflowCondition
	}{
		"update existing condition": {
			ExistingConditions: []v1alpha1.WorkflowCondition{
				{Type: v1alpha1.ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: v1alpha1.ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			WantConditions: []v1alpha1.WorkflowCondition{
				{Type: v1alpha1.ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
				{Type: v1alpha1.ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			Condition: v1alpha1.WorkflowCondition{Type: v1alpha1.ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
		},
		"append new condition": {
			ExistingConditions: []v1alpha1.WorkflowCondition{
				{Type: v1alpha1.ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
			},
			WantConditions: []v1alpha1.WorkflowCondition{
				{Type: v1alpha1.ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: v1alpha1.ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
			},
			Condition: v1alpha1.WorkflowCondition{Type: v1alpha1.ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := &v1alpha1.WorkflowStatus{
				Conditions: tt.ExistingConditions,
			}
			w.SetCondition(tt.Condition)
			if !cmp.Equal(tt.WantConditions, w.Conditions) {
				t.Errorf("SetCondition() mismatch (-want +got):\n%s", cmp.Diff(tt.WantConditions, w.Conditions))
			}
		})
	}
}
