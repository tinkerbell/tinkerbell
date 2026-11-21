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
	// lagGets makes that many Gets of a Job after each create return NotFound,
	// as a cache that hasn't seen the new Job yet would.
	lagGets int
	lagging int
	// getErr, when set, makes every Get of a Job fail with it.
	getErr error
}

func (r *jobRecorder) funcs() interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			j, ok := obj.(*bmc.Job)
			if !ok {
				return c.Create(ctx, obj, opts...)
			}
			j.SetUID(types.UID(fmt.Sprintf("job-%d", r.created+1)))
			if err := c.Create(ctx, obj, opts...); err != nil {
				return err
			}
			r.created++
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
			if _, ok := obj.(*bmc.Job); ok && r.getErr != nil {
				return r.getErr
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

// leftoverJob is the netboot Job left behind by an earlier Workflow of the same name.
func leftoverJob() *bmc.Job {
	return &bmc.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobNameNetboot.String(),
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

// A Job of the same name that another Workflow created after the clean up
// phase is not adopted: createJob deletes it and requeues, and the Workflow
// doesn't report a Job it doesn't have.
func TestCreateJobDoesNotAdoptOtherWorkflowsJob(t *testing.T) {
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, leftoverJob())
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	if _, err := s.handleJob(context.Background(), actions, jobNameNetboot); err != nil {
		t.Fatalf("handleJob: %v", err)
	}

	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "" {
		t.Errorf("adopted Job UID %q, want none", got)
	}
	if len(s.workflow.Status.Conditions) != 0 {
		t.Errorf("conditions = %+v, want none", s.workflow.Status.Conditions)
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
	s, raw := newRaceState(t, rec, leftoverJob())

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
	s, raw := newRaceState(t, rec, leftoverJob(), task)
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	if _, _, err := s.createJob(context.Background(), actions, jobNameNetboot); err != nil {
		t.Fatalf("createJob: %v", err)
	}

	if err := raw.Get(context.Background(), client.ObjectKeyFromObject(task), &bmc.Task{}); !apierrors.IsNotFound(err) {
		t.Errorf("leftover Task still exists (err %v)", err)
	}
}

// While the Job of this name is being deleted, the Workflow waits: it doesn't
// adopt the Job, nor report one.
func TestHandleJobWaitsForDeletingJob(t *testing.T) {
	deleting := leftoverJob()
	deleting.Finalizers = []string{"test/finalizer"}
	deleting.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	rec := &jobRecorder{}
	s, _ := newRaceState(t, rec, deleting)
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	r, err := s.handleJob(context.Background(), actions, jobNameNetboot)
	if err != nil {
		t.Fatalf("handleJob: %v", err)
	}
	if !r.Requeue {
		t.Errorf("result %+v does not requeue", r)
	}
	if got := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()].UID; got != "" {
		t.Errorf("adopted Job UID %q, want none", got)
	}
	if len(s.workflow.Status.Conditions) != 0 {
		t.Errorf("conditions = %+v, want none", s.workflow.Status.Conditions)
	}
}

// An error reading the Job fails the Workflow: creating the Job anyway would
// hit "already exists" if it is there, which requeues and hides the error.
func TestHandleJobFailsWhenJobCannotBeRead(t *testing.T) {
	rec := &jobRecorder{getErr: apierrors.NewForbidden(bmc.GroupVersion.WithResource("jobs").GroupResource(), jobNameNetboot.String(), fmt.Errorf("denied"))}
	s, _ := newRaceState(t, rec, leftoverJob())
	s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()] = v1alpha1.JobStatus{ExistingJobDeleted: true}

	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}
	if _, err := s.handleJob(context.Background(), actions, jobNameNetboot); err == nil {
		t.Fatal("handleJob succeeded, want an error")
	}
	if len(rec.events) != 0 {
		t.Errorf("Job events = %v, want none", rec.events)
	}
	if !s.workflow.Status.HasCondition(v1alpha1.BootJobSetupFailed, metav1.ConditionTrue) {
		t.Errorf("conditions = %+v, want %s", s.workflow.Status.Conditions, v1alpha1.BootJobSetupFailed)
	}
}
