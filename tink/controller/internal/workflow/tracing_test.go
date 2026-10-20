package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const traceTestUID = types.UID("0af76519-16cd-43dd-8448-eb211c80319c")

var traceT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func at(sec int) *metav1.Time {
	return &metav1.Time{Time: traceT0.Add(time.Duration(sec) * time.Second)}
}

func traceWorkflow(state v1alpha1.WorkflowState) *v1alpha1.Workflow {
	return &v1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "default", UID: traceTestUID},
		Spec: v1alpha1.WorkflowSpec{
			TemplateRef: "tpl",
			HardwareRef: "machine1",
			BootOptions: v1alpha1.BootOptions{BootMode: v1alpha1.BootModeNetboot},
		},
		Status: v1alpha1.WorkflowStatus{
			State: state,
			BootOptions: v1alpha1.BootOptionsStatus{
				Jobs: map[string]v1alpha1.JobStatus{"netboot-wf": {UID: "job-uid", Complete: true}},
			},
			Conditions: []v1alpha1.WorkflowCondition{
				{Type: v1alpha1.TemplateRenderedSuccess, Status: metav1.ConditionTrue, Time: at(-30)},
				{Type: v1alpha1.ToggleAllowNetbootFalse, Status: metav1.ConditionTrue, Time: at(300)},
			},
			Tasks: []v1alpha1.Task{{
				ID: "task-1", Name: "provision", AgentID: "agent-1",
				Actions: []v1alpha1.Action{
					{ID: "a1", Name: "stream", Image: "img", State: v1alpha1.WorkflowStateSuccess, ExecutionStart: at(100), ExecutionStop: at(200)},
					{ID: "a2", Name: "reboot", Image: "img", State: v1alpha1.WorkflowStateSuccess, ExecutionStart: at(200), ExecutionStop: at(250)},
				},
			}},
			CurrentState: &v1alpha1.CurrentState{ActionName: "reboot", State: v1alpha1.WorkflowStateSuccess},
		},
	}
}

func traceJob(failed bool) bmc.Job {
	j := bmc.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "netboot-wf", Namespace: "default", UID: "job-uid"},
		Status:     bmc.JobStatus{StartTime: at(0), CompletionTime: at(20)},
	}
	if failed {
		j.Status.CompletionTime = nil
		j.Status.Conditions = []bmc.JobCondition{{Type: bmc.JobFailed, Status: bmc.ConditionTrue}}
	}
	return j
}

func spanAttr(s tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, kv := range s.Attributes {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func spanNames(spans []tracetest.SpanStub) map[string]int {
	n := map[string]int{}
	for _, s := range spans {
		n[s.Name]++
	}
	return n
}

func TestWorkflowSpans(t *testing.T) {
	now := traceT0.Add(time.Hour)
	root, _ := tinkotel.WorkflowRootSpanContext(string(traceTestUID))

	t.Run("success", func(t *testing.T) {
		spans := workflowSpans(traceWorkflow(v1alpha1.WorkflowStateSuccess), []bmc.Job{traceJob(false)}, nil, now)
		if got := spanNames(spans); got["tinkerbell.workflow"] != 1 || got["tinkerbell.bmc.job"] != 1 || got["tinkerbell.action"] != 2 {
			t.Fatalf("spans = %v", got)
		}
		r := spans[0]
		if !r.SpanContext.Equal(root) || r.Parent.IsValid() {
			t.Fatalf("root span context = %v, parent = %v", r.SpanContext, r.Parent)
		}
		if !r.StartTime.Equal(at(0).Time) || !r.EndTime.Equal(at(300).Time) {
			t.Fatalf("root = %v → %v, want first job start → last condition", r.StartTime, r.EndTime)
		}
		if r.Status.Code != codes.Unset {
			t.Fatalf("status = %v", r.Status)
		}
		for key, want := range map[string]attribute.Value{
			"tinkerbell.workflow.result":            attribute.StringValue("success"),
			"tinkerbell.workflow.uid":               attribute.StringValue(string(traceTestUID)),
			"tinkerbell.hardware.name":              attribute.StringValue("machine1"),
			"tinkerbell.workflow.before_actions_ms": attribute.Int64Value(100_000),
			"tinkerbell.workflow.actions_ms":        attribute.Int64Value(150_000),
			"tinkerbell.workflow.after_actions_ms":  attribute.Int64Value(50_000),
			"tinkerbell.workflow.action.count":      attribute.IntValue(2),
		} {
			if got, ok := spanAttr(r, key); !ok || got != want {
				t.Errorf("%s = %v, want %v", key, got.Emit(), want.Emit())
			}
		}
		if _, ok := spanAttr(r, "tinkerbell.workflow.current_action"); ok {
			t.Error("current_action should not be set on success")
		}
		for _, s := range spans[1:] {
			if s.Parent.SpanID() != root.SpanID() || s.SpanContext.TraceID() != root.TraceID() {
				t.Errorf("%s is not a child of the root", s.Name)
			}
		}
	})

	t.Run("no bootMode starts at the first action", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		w.Spec.BootOptions = v1alpha1.BootOptions{}
		w.Status.BootOptions.Jobs = nil
		spans := workflowSpans(w, nil, nil, now)
		if !spans[0].StartTime.Equal(at(100).Time) {
			t.Fatalf("root start = %v, want the first action's start", spans[0].StartTime)
		}
	})

	t.Run("failed job has no span and is named on the root", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateFailed)
		w.Status.Tasks = nil
		spans := workflowSpans(w, []bmc.Job{traceJob(true)}, nil, now)
		if got := spanNames(spans); got["tinkerbell.bmc.job"] != 0 {
			t.Fatalf("spans = %v", got)
		}
		if got, _ := spanAttr(spans[0], "tinkerbell.workflow.failed_bmc_job"); got.AsString() != "netboot-wf" {
			t.Fatalf("failed_bmc_job = %q", got.AsString())
		}
		if !spans[0].StartTime.Equal(at(0).Time) || spans[0].Status.Code != codes.Error {
			t.Fatalf("root = %+v", spans[0])
		}
	})

	t.Run("agent timeout ends at the action stop", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateTimeout)
		w.Status.Conditions = nil
		w.Status.GlobalExecutionStop = at(5000)
		w.Status.Tasks[0].Actions[1].State = v1alpha1.WorkflowStateTimeout
		w.Status.CurrentState = &v1alpha1.CurrentState{ActionName: "reboot", State: v1alpha1.WorkflowStateTimeout}
		spans := workflowSpans(w, nil, nil, now)
		if !spans[0].EndTime.Equal(at(250).Time) {
			t.Fatalf("root end = %v, want the action stop", spans[0].EndTime)
		}
		if got, _ := spanAttr(spans[0], "tinkerbell.workflow.result"); got.AsString() != "timeout" {
			t.Fatalf("result = %q", got.AsString())
		}
		if got, _ := spanAttr(spans[0], "tinkerbell.workflow.current_action"); got.AsString() != "reboot" {
			t.Fatalf("current_action = %q", got.AsString())
		}
	})

	t.Run("global timeout ends at the deadline and skips the running action", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateTimeout)
		w.Status.Conditions = nil
		w.Status.GlobalExecutionStop = at(1000)
		w.Status.Tasks[0].Actions[1] = v1alpha1.Action{ID: "a2", Name: "reboot", State: v1alpha1.WorkflowStateRunning}
		w.Status.CurrentState = &v1alpha1.CurrentState{ActionName: "reboot", State: v1alpha1.WorkflowStateRunning}
		spans := workflowSpans(w, nil, nil, now)
		if !spans[0].EndTime.Equal(at(1000).Time) {
			t.Fatalf("root end = %v, want the global deadline", spans[0].EndTime)
		}
		if got := spanNames(spans); got["tinkerbell.action"] != 1 {
			t.Fatalf("spans = %v, want only the finished action", got)
		}
	})

	t.Run("nothing ran gives a zero-length root", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateFailed)
		w.Status.Tasks = nil
		w.Status.BootOptions.Jobs = nil
		spans := workflowSpans(w, nil, nil, now)
		if len(spans) != 1 || !spans[0].StartTime.Equal(spans[0].EndTime) {
			t.Fatalf("spans = %+v", spans)
		}
	})

	t.Run("incoming traceparent becomes a link", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		w.Annotations = map[string]string{traceparentAnnotation: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
		spans := workflowSpans(w, nil, nil, now)
		if len(spans[0].Links) != 1 || spans[0].Links[0].SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("links = %+v", spans[0].Links)
		}
		if spans[0].Parent.IsValid() {
			t.Fatal("incoming context must not be the parent")
		}
	})

	t.Run("hardware attributes", func(t *testing.T) {
		hw := &v1alpha1.Hardware{Status: v1alpha1.HardwareStatus{Attributes: &v1alpha1.HardwareAttributes{
			OutOfBand: &v1alpha1.Attributes{
				Product: &v1alpha1.Product{Vendor: "Dell", Model: "R650"},
				BMC:     &v1alpha1.BMC{Vendor: "Dell", FirmwareVersion: "7.0"},
			},
		}}}
		spans := workflowSpans(traceWorkflow(v1alpha1.WorkflowStateSuccess), nil, hw, now)
		if got, _ := spanAttr(spans[0], "tinkerbell.hardware.model"); got.AsString() != "R650" {
			t.Fatalf("model = %q", got.AsString())
		}
	})

	t.Run("uid that isn't a uuid", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		w.UID = "nope"
		if spans := workflowSpans(w, nil, nil, now); spans != nil {
			t.Fatalf("expected no spans, got %d", len(spans))
		}
	})
}

func TestReconcileSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	w := traceWorkflow(v1alpha1.WorkflowStatePending)
	c := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w).WithStatusSubresource(w).Build()
	r := &Reconciler{client: c, nowFunc: time.Now, references: fakeReferences{}}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "wf", Namespace: "default"}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 || spans[0].Name != "tinkerbell.workflow.reconcile" {
		t.Fatalf("spans = %+v", spans)
	}
	s := spans[0]
	root, _ := tinkotel.WorkflowRootSpanContext(string(traceTestUID))
	if len(s.Links) != 1 || !s.Links[0].SpanContext.Equal(root) {
		t.Fatalf("links = %+v, want the workflow root", s.Links)
	}
	if s.SpanContext.TraceID() == root.TraceID() {
		t.Fatal("reconcile span must be its own trace, not part of the workflow trace")
	}
	stub := tracetest.SpanStubFromReadOnlySpan(s.Snapshot())
	for key, want := range map[string]string{
		"tinkerbell.workflow.state_before": "PENDING",
		"tinkerbell.workflow.state_after":  "PENDING",
		"tinkerbell.hardware.name":         "machine1",
	} {
		if got, _ := spanAttr(stub, key); got.AsString() != want {
			t.Errorf("%s = %q, want %q", key, got.AsString(), want)
		}
	}
}

func traceScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = bmc.AddToScheme(s)
	return s
}

func newTraceReconciler(c client.Client, exp *tracetest.InMemoryExporter, now time.Time) *Reconciler {
	return &Reconciler{
		client:     c,
		nowFunc:    func() time.Time { return now },
		references: fakeReferences{},
		emitter:    tinkotel.NewEmitter(sdktrace.NewSimpleSpanProcessor(exp), resource.Empty()),
	}
}

func TestEmitTrace(t *testing.T) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "wf", Namespace: "default"}}
	soon := traceT0.Add(10 * time.Minute)

	t.Run("sends once and sets the condition", func(t *testing.T) {
		job := traceJob(false)
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		c := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w, &job).WithStatusSubresource(w).Build()
		exp := tracetest.NewInMemoryExporter()
		r := newTraceReconciler(c, exp, soon)

		for range 2 {
			if _, err := r.Reconcile(context.Background(), req); err != nil {
				t.Fatal(err)
			}
		}
		if got := spanNames(exp.GetSpans()); got["tinkerbell.workflow"] != 1 || got["tinkerbell.bmc.job"] != 1 || got["tinkerbell.action"] != 2 {
			t.Fatalf("spans = %v", got)
		}
		got := &v1alpha1.Workflow{}
		if err := c.Get(context.Background(), req.NamespacedName, got); err != nil {
			t.Fatal(err)
		}
		if !got.Status.HasCondition(v1alpha1.TraceEmitted, metav1.ConditionTrue) {
			t.Fatal("expected the TraceEmitted condition")
		}
	})

	t.Run("tracing off writes nothing", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		c := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w).WithStatusSubresource(w).Build()
		r := &Reconciler{client: c, nowFunc: func() time.Time { return soon }, references: fakeReferences{}}
		if _, err := r.Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		got := &v1alpha1.Workflow{}
		_ = c.Get(context.Background(), req.NamespacedName, got)
		if got.Status.HasCondition(v1alpha1.TraceEmitted, metav1.ConditionTrue) {
			t.Fatal("condition written with tracing off")
		}
	})

	t.Run("patch conflict sends nothing", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateFailed)
		base := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w).WithStatusSubresource(w).Build()
		c := interceptor.NewClient(base, interceptor.Funcs{
			SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
				return kerrors.NewConflict(schema.GroupResource{Resource: "workflows"}, "wf", errors.New("conflict"))
			},
		})
		exp := tracetest.NewInMemoryExporter()
		if _, err := newTraceReconciler(c, exp, soon).Reconcile(context.Background(), req); !kerrors.IsConflict(err) {
			t.Fatalf("err = %v, want conflict", err)
		}
		if n := len(exp.GetSpans()); n != 0 {
			t.Fatalf("sent %d spans after a failed patch", n)
		}
	})

	t.Run("workflow that ended long ago is skipped", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStateSuccess)
		c := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w).WithStatusSubresource(w).Build()
		exp := tracetest.NewInMemoryExporter()
		if _, err := newTraceReconciler(c, exp, traceT0.Add(48*time.Hour)).Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		got := &v1alpha1.Workflow{}
		_ = c.Get(context.Background(), req.NamespacedName, got)
		if len(exp.GetSpans()) != 0 || got.Status.HasCondition(v1alpha1.TraceEmitted, metav1.ConditionTrue) {
			t.Fatal("expected nothing for an old workflow")
		}
	})

	t.Run("pending is not terminal", func(t *testing.T) {
		w := traceWorkflow(v1alpha1.WorkflowStatePending)
		c := fake.NewClientBuilder().WithScheme(traceScheme()).WithObjects(w).WithStatusSubresource(w).Build()
		exp := tracetest.NewInMemoryExporter()
		if _, err := newTraceReconciler(c, exp, soon).Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if n := len(exp.GetSpans()); n != 0 {
			t.Fatalf("sent %d spans for a pending workflow", n)
		}
	})
}
