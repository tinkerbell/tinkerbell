package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHandleCustombootActions_EmptyAndAlreadyDone(t *testing.T) {
	s := &state{workflow: &v1alpha1.Workflow{}, client: newWebhookTestClient()}

	if r, done, err := s.handleCustombootActions(context.Background(), nil, "list", jobName("base")); err != nil || !done || r != (reconcile.Result{}) {
		t.Fatalf("empty list: got result=%v done=%v err=%v", r, done, err)
	}

	wf := &v1alpha1.Workflow{
		Status: v1alpha1.WorkflowStatus{
			BootOptions: v1alpha1.BootOptionsStatus{
				Actions: map[string]v1alpha1.ActionListStatus{"list": {Completed: 1}},
			},
		},
	}
	s2 := &state{workflow: wf, client: newWebhookTestClient()}
	actions := []v1alpha1.CustombootAction{{Action: bmc.Action{PowerAction: toPtr(bmc.PowerHardOff)}}}
	if _, done, err := s2.handleCustombootActions(context.Background(), actions, "list", jobName("base")); err != nil || !done {
		t.Fatalf("already-done: got done=%v err=%v", done, err)
	}
}

func TestHandleCustombootActions_WebhookEntry(t *testing.T) {
	hw := &v1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"}}
	wf := &v1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}
	wf.Spec.HardwareRef = "test-hardware"

	t.Run("success advances Completed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer srv.Close()

		s := &state{workflow: wf.DeepCopy(), client: newWebhookTestClient(hw)}
		actions := []v1alpha1.CustombootAction{{Webhook: &v1alpha1.WebhookAction{URL: srv.URL}}}
		r, done, err := s.handleCustombootActions(context.Background(), actions, "list", jobName("base"))
		if err != nil || !done {
			t.Fatalf("got done=%v err=%v", done, err)
		}
		if diff := cmp.Diff(reconcile.Result{Requeue: true}, r); diff != "" {
			t.Errorf("unexpected result (-want +got):\n%s", diff)
		}
		if got := s.workflow.Status.BootOptions.Actions["list"].Completed; got != 1 {
			t.Errorf("want Completed=1, got %d", got)
		}
	})

	t.Run("failure does not advance Completed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
		defer srv.Close()

		s := &state{workflow: wf.DeepCopy(), client: newWebhookTestClient(hw)}
		actions := []v1alpha1.CustombootAction{{Webhook: &v1alpha1.WebhookAction{URL: srv.URL}}}
		_, done, err := s.handleCustombootActions(context.Background(), actions, "list", jobName("base"))
		if err == nil || done {
			t.Fatalf("got done=%v err=%v, want error and done=false", done, err)
		}
		if got := s.workflow.Status.BootOptions.Actions["list"].Completed; got != 0 {
			t.Errorf("want Completed=0 after failure, got %d", got)
		}
	})
}

func TestHandleCustombootActions_BMCEntryCreatesJobWhenNoneExists(t *testing.T) {
	wf := &v1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}
	wf.Spec.HardwareRef = "test-hardware"
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"},
		Spec: v1alpha1.HardwareSpec{
			BMCRef: &corev1.TypedLocalObjectReference{Name: "test-bmc", Kind: "machine.bmc.tinkerbell.org"},
		},
	}
	s := &state{workflow: wf, client: newWebhookTestClient(hw)}
	actions := []v1alpha1.CustombootAction{{Action: bmc.Action{PowerAction: toPtr(bmc.PowerHardOff)}}}

	r, done, err := s.handleCustombootActions(context.Background(), actions, "list", jobName("base"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("want done=false while the batch's Job is still being created")
	}
	if diff := cmp.Diff(reconcile.Result{Requeue: true}, r); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}
	status := s.workflow.Status.BootOptions.Actions["list"]
	if status.Completed != 0 {
		t.Errorf("want Completed=0, got %d", status.Completed)
	}

	// The batch Job name ("base-0", derived from baseName+Completed, never stored) must have
	// actually been created — this is the create-a-new-Job path's whole point.
	job := &bmc.Job{}
	if err := s.client.Get(context.Background(), client.ObjectKey{Name: "base-0", Namespace: "default"}, job); err != nil {
		t.Fatalf("expected bmc.Job %q to have been created: %v", "base-0", err)
	}
}

// TestHandleCustombootActions_MixedSequence drives [BMC, BMC, Webhook, BMC] across four
// reconcile-like calls, verifying: contiguous BMC entries batch into one Job (entries 0-1), the
// batch boundary stops at the webhook (entry 2), and the trailing BMC entry (entry 3) gets its
// own batch/Job — each with a distinct, deterministically-named bmc.Job.
func TestHandleCustombootActions_MixedSequence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()

	actions := []v1alpha1.CustombootAction{
		{Action: bmc.Action{PowerAction: toPtr(bmc.PowerHardOff)}},
		{Action: bmc.Action{PowerAction: toPtr(bmc.PowerOn)}},
		{Webhook: &v1alpha1.WebhookAction{URL: srv.URL}},
		{Action: bmc.Action{PowerAction: toPtr(bmc.PowerCycle)}},
	}
	base := jobName("mixed")
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"},
		Spec: v1alpha1.HardwareSpec{
			BMCRef: &corev1.TypedLocalObjectReference{Name: "test-bmc", Kind: "machine.bmc.tinkerbell.org"},
		},
	}
	wf := &v1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}
	wf.Spec.HardwareRef = "test-hardware"

	completedJob := func(name string) *bmc.Job {
		return &bmc.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("uid-" + name)},
			Status:     bmc.JobStatus{Conditions: []bmc.JobCondition{{Type: bmc.JobCompleted, Status: bmc.ConditionTrue}}},
		}
	}

	// Call 1: batch [0,2) ("mixed-0") is already complete — Completed jumps to 2.
	wf.Status = v1alpha1.WorkflowStatus{
		BootOptions: v1alpha1.BootOptionsStatus{
			Jobs: map[string]v1alpha1.JobStatus{
				"mixed-0": {ExistingJobDeleted: true, UID: types.UID("uid-mixed-0"), Complete: true},
			},
		},
	}
	s := &state{workflow: wf, client: newWebhookTestClient(hw, completedJob("mixed-0"))}
	_, done, err := s.handleCustombootActions(context.Background(), actions, "list", base)
	if err != nil {
		t.Fatalf("call 1: unexpected error: %v", err)
	}
	if done {
		t.Fatal("call 1: want done=false (2 of 4 entries done)")
	}
	status := s.workflow.Status.BootOptions.Actions["list"]
	if status.Completed != 2 {
		t.Fatalf("call 1: want Completed=2, got %d", status.Completed)
	}

	// Call 2: entry 2 is the webhook — Completed advances to 3.
	_, done, err = s.handleCustombootActions(context.Background(), actions, "list", base)
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if done {
		t.Fatal("call 2: want done=false (3 of 4 entries done)")
	}
	if got := s.workflow.Status.BootOptions.Actions["list"].Completed; got != 3 {
		t.Fatalf("call 2: want Completed=3, got %d", got)
	}

	// Call 3: entry 3 is a lone BMC entry — a new batch/Job ("mixed-3", derived from
	// baseName+Completed) gets created and isn't complete yet.
	_, done, err = s.handleCustombootActions(context.Background(), actions, "list", base)
	if err != nil {
		t.Fatalf("call 3: unexpected error: %v", err)
	}
	if done {
		t.Fatal("call 3: want done=false (batch 2 just created)")
	}
	status = s.workflow.Status.BootOptions.Actions["list"]
	if status.Completed != 3 {
		t.Fatalf("call 3: want Completed=3, got %d", status.Completed)
	}

	// Call 4: "mixed-3" is now complete — Completed reaches 4, done=true. Rebuild the client
	// with the completed Job pre-seeded (fake client + status subresources don't mix well
	// with a mid-test Create of an object whose Status is already populated); the in-memory
	// s.workflow.Status carries over unchanged across the swap.
	s.workflow.Status.BootOptions.Jobs["mixed-3"] = v1alpha1.JobStatus{ExistingJobDeleted: true, UID: types.UID("uid-mixed-3"), Complete: true}
	s.client = newWebhookTestClient(hw, completedJob("mixed-3"))
	_, done, err = s.handleCustombootActions(context.Background(), actions, "list", base)
	if err != nil {
		t.Fatalf("call 4: unexpected error: %v", err)
	}
	if !done {
		t.Fatal("call 4: want done=true (all 4 entries done)")
	}
	status = s.workflow.Status.BootOptions.Actions["list"]
	if status.Completed != 4 {
		t.Fatalf("call 4: want Completed=4, got %d", status.Completed)
	}
}
