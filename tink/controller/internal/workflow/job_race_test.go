package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// jobRecorder gives every created Job a unique UID (the fake client doesn't)
// and records the Job creates and deletes that succeeded.
type jobRecorder struct {
	created int
	events  []string
	// staleGets, when > 0, makes that many Gets of a deleted Job return the
	// deleted copy, as a lagging informer cache would.
	staleGets int
	deleted   *bmc.Job
	// failGets, when > 0, makes that many Gets of a Job fail with a server
	// timeout, as a passing API error would.
	failGets int
	// lagGets makes that many Gets of a Job after each create return NotFound,
	// as a cache that hasn't seen the new Job yet would.
	lagGets int
	lagging int
}

func (r *jobRecorder) funcs() interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			j, ok := obj.(*bmc.Job)
			if !ok {
				return c.Create(ctx, obj, opts...)
			}
			// Return the API server's error for an existing name: the fake
			// client fails with a ManagedFields error instead.
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), &bmc.Job{}); err == nil {
				return apierrors.NewAlreadyExists(bmc.GroupVersion.WithResource("jobs").GroupResource(), j.Name)
			}
			r.created++
			j.SetUID(types.UID(fmt.Sprintf("job-%d", r.created)))
			if err := c.Create(ctx, obj, opts...); err != nil {
				return err
			}
			r.events = append(r.events, "create "+string(j.UID))
			r.lagging = r.lagGets
			return nil
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*bmc.Job); !ok {
				return c.Delete(ctx, obj, opts...)
			}
			existing := &bmc.Job{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
				return c.Delete(ctx, obj, opts...)
			}
			if err := c.Delete(ctx, obj, opts...); err != nil {
				return err
			}
			r.events = append(r.events, "delete "+string(existing.UID))
			r.deleted = existing
			return nil
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*bmc.Job); ok && r.failGets > 0 {
				r.failGets--
				return apierrors.NewServerTimeout(bmc.GroupVersion.WithResource("jobs").GroupResource(), "get", 1)
			}
			if _, ok := obj.(*bmc.Job); ok && r.lagging > 0 {
				r.lagging--
				return apierrors.NewNotFound(bmc.GroupVersion.WithResource("jobs").GroupResource(), key.Name)
			}
			if j, ok := obj.(*bmc.Job); ok && r.staleGets > 0 && r.deleted != nil && r.deleted.Name == key.Name {
				r.staleGets--
				r.deleted.DeepCopyInto(j)
				return nil
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}

// newRaceState returns a state whose client, which stands for the cached
// client, goes through rec, and the raw fake client, which stands for the API
// server.
func newRaceState(t *testing.T, rec *jobRecorder, objs ...client.Object) (*state, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := bmc.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"},
		Spec: v1alpha1.HardwareSpec{
			BMCRef: &v1.TypedLocalObjectReference{Name: "test-bmc", Kind: "machine.bmc.tinkerbell.org"},
		},
	}
	wf := &v1alpha1.Workflow{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Workflow"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default", UID: "workflow-uid"},
		Spec:       v1alpha1.WorkflowSpec{HardwareRef: "test-hardware"},
		Status: v1alpha1.WorkflowStatus{
			BootOptions: v1alpha1.BootOptionsStatus{Jobs: map[string]v1alpha1.JobStatus{}},
		},
	}
	raw := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&bmc.Job{}, &v1alpha1.Workflow{}).WithObjects(append(objs, hw, wf)...).Build()
	c := interceptor.NewClient(raw, rec.funcs())
	return &state{client: c, workflow: wf}, raw
}

// ownJob is a Job of the given name created by the test Workflow.
func ownJob(name jobName, uid types.UID) *bmc.Job {
	j := leftoverJob(name)
	j.UID = uid
	j.OwnerReferences[0].UID = "workflow-uid"
	return j
}

// jobFailed reports whether the Workflow has a boot job failure condition.
func jobFailed(s *state) bool {
	for _, c := range s.workflow.Status.Conditions {
		if (c.Type == v1alpha1.BootJobFailed || c.Type == v1alpha1.BootJobSetupFailed) && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// leftoverJob is a Job of the given name left behind by an earlier Workflow.
func leftoverJob(name jobName) *bmc.Job {
	return &bmc.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.String(),
			Namespace: "default",
			UID:       "leftover",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "Workflow",
				Name:       "test-workflow",
				UID:        "earlier-workflow-uid",
				Controller: toPtr(true),
			}},
		},
	}
}

// runUntilTracking calls handleJob for the netboot Job until the Workflow's
// status says it is tracking it: the next pass would call trackRunningJob.
func runUntilTracking(t *testing.T, s *state) {
	t.Helper()
	name := jobNameNetboot
	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	for range 10 {
		j, found := s.workflow.Status.BootOptions.Jobs[name.String()]
		if found && j.ExistingJobDeleted && j.UID != "" {
			return
		}
		if _, err := s.handleJob(context.Background(), actions, name); err != nil {
			t.Fatalf("handleJob: %v", err)
		}
	}
	t.Fatalf("handleJob never reached tracking: status %+v", s.workflow.Status.BootOptions.Jobs[name.String()])
}

// A Workflow must never delete the boot Job it created itself: Rufio may
// already be running it.
func TestHandleJobDoesNotDeleteOwnJob(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec)

	runUntilTracking(t, s)

	want := []string{"create job-1"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Errorf("Job events = %v, want %v", rec.events, want)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "job-1" {
		t.Errorf("tracked Job UID = %q, want %q", got, "job-1")
	}
}

// A Job left behind by an earlier Workflow with the same name is deleted
// before the new one is created.
func TestHandleJobDeletesLeftoverJob(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, leftoverJob(jobNameNetboot))

	runUntilTracking(t, s)

	want := []string{"delete leftover", "create job-1"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Errorf("Job events = %v, want %v", rec.events, want)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "job-1" {
		t.Errorf("tracked Job UID = %q, want %q", got, "job-1")
	}
}

// A Job of the same name that another Workflow created after the clean up
// phase is not adopted: createJob deletes it and requeues.
func TestCreateJobDoesNotAdoptOtherWorkflowsJob(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, leftoverJob(jobNameNetboot))
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	if _, err := s.createJob(context.Background(), actions, jobNameNetboot); err != nil {
		t.Fatalf("createJob: %v", err)
	}

	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "" {
		t.Errorf("adopted Job UID %q, want none", got)
	}
	want := []string{"delete leftover"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Errorf("Job events = %v, want %v", rec.events, want)
	}
}

// When the cache still shows a Job that was just deleted, the Workflow must
// not adopt it and then track a Job that no longer exists.
func TestHandleJobDoesNotAdoptDeletedJob(t *testing.T) {
	rec := &jobRecorder{staleGets: 1}
	s, raw := newRaceState(t, rec, leftoverJob(jobNameNetboot))

	runUntilTracking(t, s)

	tracked := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID
	live := &bmc.Job{}
	if err := raw.Get(context.Background(), client.ObjectKey{Name: jobNameNetboot.String(), Namespace: "default"}, live); err != nil {
		t.Fatalf("tracked Job %q does not exist: %v (events %v)", tracked, err, rec.events)
	}
	if live.UID != tracked {
		t.Errorf("tracked Job UID = %q, live Job UID = %q (events %v)", tracked, live.UID, rec.events)
	}
}

// A passing API error while tracking a running Job requeues: it must not fail
// the Workflow, since Rufio goes on running the Job.
func TestTrackRunningJobRequeuesOnTransientError(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, ownJob(jobNameNetboot, "job-1"))
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true, UID: "job-1"}
	rec.failGets = 1

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	r, err := s.handleJob(context.Background(), actions, jobNameNetboot)
	if err != nil {
		t.Errorf("handleJob: %v", err)
	}
	if jobFailed(s) {
		t.Errorf("Workflow has a job failure condition: %+v", s.workflow.Status.Conditions)
	}
	if !r.Requeue && r.RequeueAfter == 0 {
		t.Errorf("result %+v does not requeue", r)
	}
}

// A passing API error on the pass that should adopt the Job must not make
// createJob create a second one, nor fail the Workflow.
func TestCreateJobTransientErrorDoesNotFail(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, ownJob(jobNameNetboot, "job-1"))
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}
	rec.failGets = 1

	runUntilTracking(t, s)

	if len(rec.events) != 0 {
		t.Errorf("Job events = %v, want none", rec.events)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "job-1" {
		t.Errorf("tracked Job UID = %q, want %q", got, "job-1")
	}
}

// When the cache hasn't seen the Job createJob just created, the next pass
// must not fail on "already exists": it waits for the cache and adopts it.
func TestCreateJobCacheLagsBehindCreate(t *testing.T) {
	rec := &jobRecorder{lagGets: 1}
	s, _ := newRaceState(t, rec)

	runUntilTracking(t, s)

	want := []string{"create job-1"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Errorf("Job events = %v, want %v", rec.events, want)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "job-1" {
		t.Errorf("tracked Job UID = %q, want %q", got, "job-1")
	}
}

// Deleting a Job createJob found from another Workflow also deletes its
// Tasks: there is no garbage collector with the embedded kube-apiserver.
func TestCreateJobDeletesTasksOfOtherWorkflowsJob(t *testing.T) {
	task := &bmc.Task{ObjectMeta: metav1.ObjectMeta{
		Name:      jobNameNetboot.String() + "-task-0",
		Namespace: "default",
		Labels:    map[string]string{"owner-name": jobNameNetboot.String()},
	}}
	rec := &jobRecorder{}
	s, raw := newRaceState(t, rec, leftoverJob(jobNameNetboot), task)
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	if _, err := s.createJob(context.Background(), actions, jobNameNetboot); err != nil {
		t.Fatalf("createJob: %v", err)
	}

	if err := raw.Get(context.Background(), client.ObjectKeyFromObject(task), &bmc.Task{}); !apierrors.IsNotFound(err) {
		t.Errorf("leftover Task still exists (err %v)", err)
	}
}

// A Job this Workflow created before the clean up phase ran, as an earlier
// tink-controller did, is kept and tracked, not deleted.
func TestHandleJobKeepsOwnJobCreatedBeforeCleanUp(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, ownJob(jobNameNetboot, "existing"))

	runUntilTracking(t, s)

	if len(rec.events) != 0 {
		t.Errorf("Job events = %v, want none", rec.events)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "existing" {
		t.Errorf("tracked Job UID = %q, want %q", got, "existing")
	}
}
