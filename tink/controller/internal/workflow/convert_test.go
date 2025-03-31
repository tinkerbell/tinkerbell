package workflow

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
)

func TestYAMLToStatus(t *testing.T) {
	cases := []struct {
		name    string
		inputWf *Workflow
		want    *v1alpha1.WorkflowStatus
	}{
		{
			"Nil workflow",
			nil,
			nil,
		},
		{
			"Full crd",
			&Workflow{
				Version:       "1",
				Name:          "debian-provision",
				ID:            "0a90fac9-b509-4aa5-b294-5944128ece81",
				GlobalTimeout: 600,
				Tasks: []Task{
					{
						Name:       "do-or-do-not-there-is-no-try",
						WorkerAddr: "00:00:53:00:53:F4",
						Actions: []Action{
							{
								Name:    "stream-image-to-disk",
								Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
								Timeout: 300,
								Volumes: []string{
									"/dev:/dev",
									"/dev/console:/dev/console",
									"/lib/firmware:/lib/firmware:ro",
									"/tmp/debug:/tmp/debug",
								},
								Environment: map[string]string{
									"COMPRESSED": "true",
									"DEST_DISK":  "/dev/nvme0n1",
									"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
								},
								Pid: "host",
							},
						},
					},
				},
			},
			&v1alpha1.WorkflowStatus{
				GlobalTimeout: 600,
				Tasks: []v1alpha1.Task{
					{
						Name:       "do-or-do-not-there-is-no-try",
						WorkerAddr: "00:00:53:00:53:F4",
						Actions: []v1alpha1.Action{
							{
								Name:    "stream-image-to-disk",
								Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
								Timeout: 300,
								Volumes: []string{
									"/dev:/dev",
									"/dev/console:/dev/console",
									"/lib/firmware:/lib/firmware:ro",
									"/tmp/debug:/tmp/debug",
								},
								Pid: "host",
								Environment: map[string]string{
									"COMPRESSED": "true",
									"DEST_DISK":  "/dev/nvme0n1",
									"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
								},
								State: "STATE_PENDING",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := YAMLToStatus(tc.inputWf)
			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreFields(v1alpha1.Task{}, "ID"), cmpopts.IgnoreFields(v1alpha1.Action{}, "ID")); diff != "" {
				t.Errorf("unexpected difference:\n%v", diff)
			}
		})
	}
}
