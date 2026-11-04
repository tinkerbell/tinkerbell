package workflow

import (
	"context"
	"crypto/rand"
	"sort"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	tracerName = "github.com/tinkerbell/tinkerbell/tink/controller"
	// traceparentAnnotation lets a Workflow's creator link the Workflow trace to its own trace.
	traceparentAnnotation = "tinkerbell.org/traceparent"
	// traceMaxAge is how long after a Workflow ends its trace is still sent.
	traceMaxAge = time.Hour
)

// emitTrace sends the Workflow trace once, after the Workflow reaches a terminal state.
// The TraceEmitted condition is patched first, with an optimistic lock, so only one
// reconcile ever sends the trace. Delivery is at most once.
func (r *Reconciler) emitTrace(ctx context.Context, stored, wflow *v1alpha1.Workflow) error {
	e := r.emitter
	if e == nil {
		e = tinkotel.DefaultEmitter()
	}
	if !e.Enabled() || wflow.Status.HasCondition(v1alpha1.TraceEmitted, metav1.ConditionTrue) {
		return nil
	}
	now := r.nowFunc()
	spans := workflowSpans(wflow, r.bmcJobs(ctx, wflow), r.hardware(ctx, wflow), now)
	if len(spans) == 0 {
		return nil
	}
	if now.Sub(spans[0].EndTime) > traceMaxAge {
		journal.Log(ctx, "workflow ended too long ago to send a trace")
		return nil
	}
	wflow.Status.SetCondition(v1alpha1.WorkflowCondition{
		Type:   v1alpha1.TraceEmitted,
		Status: metav1.ConditionTrue,
		Time:   &metav1.Time{Time: now},
	})
	if err := mergePatchStatus(ctx, r.client, stored, wflow); err != nil {
		return err
	}
	e.Emit(tracerName, spans)
	journal.Log(ctx, "workflow trace sent")

	return nil
}

// bmcJobs returns the BMC Jobs this Workflow created that still exist.
func (r *Reconciler) bmcJobs(ctx context.Context, w *v1alpha1.Workflow) []bmc.Job {
	names := make([]string, 0, len(w.Status.BootOptions.Jobs))
	for name := range w.Status.BootOptions.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	jobs := make([]bmc.Job, 0, len(names))
	for _, name := range names {
		j := bmc.Job{}
		if err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: w.Namespace}, &j); err != nil {
			continue
		}
		if j.UID == w.Status.BootOptions.Jobs[name].UID {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// hardware returns the Workflow's Hardware, or nil if it has none or can't be read.
func (r *Reconciler) hardware(ctx context.Context, w *v1alpha1.Workflow) *v1alpha1.Hardware {
	if w.Spec.HardwareRef == "" {
		return nil
	}
	hw := &v1alpha1.Hardware{}
	if err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: w.Spec.HardwareRef, Namespace: w.Namespace}, hw); err != nil {
		return nil
	}
	return hw
}

// workflowSpans builds the Workflow trace from timestamps recorded in Kubernetes.
// The root span is first. It returns nil if the Workflow UID isn't a UUID.
func workflowSpans(w *v1alpha1.Workflow, jobs []bmc.Job, hw *v1alpha1.Hardware, now time.Time) []tracetest.SpanStub {
	root, ok := tinkotel.WorkflowRootSpanContext(string(w.UID))
	if !ok {
		return nil
	}
	common := []attribute.KeyValue{
		attribute.String("tinkerbell.workflow.uid", string(w.UID)),
		attribute.String("tinkerbell.workflow.name", w.Name),
		attribute.String("tinkerbell.workflow.namespace", w.Namespace),
	}
	if w.Spec.HardwareRef != "" {
		common = append(common,
			attribute.String("tinkerbell.hardware.name", w.Spec.HardwareRef),
			attribute.String("tinkerbell.hardware.namespace", w.Namespace),
		)
	}
	child := func(name string, start, end time.Time, attrs ...attribute.KeyValue) tracetest.SpanStub {
		return tracetest.SpanStub{
			Name:        name,
			SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: root.TraceID(), SpanID: newSpanID()}),
			Parent:      root,
			SpanKind:    trace.SpanKindInternal,
			StartTime:   start,
			EndTime:     end,
			Attributes:  append(append([]attribute.KeyValue{}, common...), attrs...),
		}
	}

	var children []tracetest.SpanStub
	var start time.Time
	earliest := func(t time.Time) {
		if start.IsZero() || t.Before(start) {
			start = t
		}
	}

	rootAttrs := []attribute.KeyValue{}
	for _, j := range jobs {
		if j.HasCondition(bmc.JobFailed, bmc.ConditionTrue) {
			rootAttrs = append(rootAttrs, attribute.String("tinkerbell.workflow.failed_bmc_job", j.Name))
		}
		if j.Status.StartTime == nil {
			continue
		}
		earliest(j.Status.StartTime.Time)
		// rufio only sets CompletionTime on success, so failed Jobs get no span.
		if j.Status.CompletionTime == nil {
			continue
		}
		children = append(children, child("tinkerbell.bmc.job", j.Status.StartTime.Time, j.Status.CompletionTime.Time,
			attribute.String("tinkerbell.bmc.job.name", j.Name)))
	}

	var firstAction, lastAction time.Time
	for _, task := range w.Status.Tasks {
		for _, a := range task.Actions {
			s, e := recorded(a.ExecutionStart), recorded(a.ExecutionStop)
			if s.IsZero() || e.IsZero() {
				continue
			}
			earliest(s)
			if firstAction.IsZero() || s.Before(firstAction) {
				firstAction = s
			}
			if e.After(lastAction) {
				lastAction = e
			}
			span := child("tinkerbell.action", s, e,
				attribute.String("tinkerbell.action.name", a.Name),
				attribute.String("tinkerbell.action.id", a.ID),
				attribute.String("tinkerbell.action.image", a.Image),
				attribute.String("tinkerbell.action.state", string(a.State)),
				attribute.String("tinkerbell.action.task.name", task.Name),
				attribute.String("tinkerbell.action.task.id", task.ID),
				attribute.String("tinkerbell.agent.id", task.AgentID),
			)
			if a.State != v1alpha1.WorkflowStateSuccess {
				span.Status = sdktrace.Status{Code: codes.Error, Description: string(a.State)}
			}
			children = append(children, span)
		}
	}

	end := endTime(w)
	if end.IsZero() {
		end = now
	}
	if start.IsZero() || start.After(end) {
		start = end
	}

	result, status := workflowResult(w.Status.State)
	rootAttrs = append(rootAttrs,
		attribute.String("tinkerbell.workflow.template", w.Spec.TemplateRef),
		attribute.String("tinkerbell.workflow.boot_mode", string(w.Spec.BootOptions.BootMode)),
		attribute.Int64("tinkerbell.workflow.global_timeout", w.Status.GlobalTimeout),
		attribute.String("tinkerbell.workflow.result", result),
		attribute.Int("tinkerbell.workflow.action.count", countActions(w)),
	)
	if cs := w.Status.CurrentState; cs != nil && result != "success" {
		rootAttrs = append(rootAttrs,
			attribute.String("tinkerbell.workflow.current_action", cs.ActionName),
			attribute.String("tinkerbell.workflow.current_action_state", string(cs.State)),
		)
	}
	if !firstAction.IsZero() {
		rootAttrs = append(rootAttrs,
			attribute.Int64("tinkerbell.workflow.before_actions_ms", firstAction.Sub(start).Milliseconds()),
			attribute.Int64("tinkerbell.workflow.actions_ms", lastAction.Sub(firstAction).Milliseconds()),
			attribute.Int64("tinkerbell.workflow.after_actions_ms", end.Sub(lastAction).Milliseconds()),
		)
	}
	rootAttrs = append(rootAttrs, hardwareAttributes(hw)...)

	rootSpan := tracetest.SpanStub{
		Name:        "tinkerbell.workflow",
		SpanContext: root,
		SpanKind:    trace.SpanKindServer,
		StartTime:   start,
		EndTime:     end,
		Attributes:  append(common, rootAttrs...),
		Status:      status,
	}
	if sc, ok := tinkotel.SpanContextFromTraceparent(w.Annotations[traceparentAnnotation]); ok {
		rootSpan.Links = []sdktrace.Link{{SpanContext: sc}}
	}

	return append([]tracetest.SpanStub{rootSpan}, children...)
}

// endTime is the latest timestamp recorded in the Workflow status.
func endTime(w *v1alpha1.Workflow) time.Time {
	var end time.Time
	later := func(t time.Time) {
		if t.After(end) {
			end = t
		}
	}
	for _, c := range w.Status.Conditions {
		if c.Type != v1alpha1.TraceEmitted {
			later(recorded(c.Time))
		}
	}
	for _, task := range w.Status.Tasks {
		for _, a := range task.Actions {
			later(recorded(a.ExecutionStop))
		}
	}
	// The controller's global timeout records no time of its own, but its deadline is the end.
	// An Agent-reported timeout sets the current state to TIMEOUT and has an Action stop time instead.
	if w.Status.State == v1alpha1.WorkflowStateTimeout && (w.Status.CurrentState == nil || w.Status.CurrentState.State != v1alpha1.WorkflowStateTimeout) {
		later(recorded(w.Status.GlobalExecutionStop))
	}
	return end
}

// recorded returns t's time, or the zero time if t is nil or was never set.
// The Agent can report a zero timestamp, which is stored as the Unix epoch or year 1.
func recorded(t *metav1.Time) time.Time {
	if t == nil || t.Unix() <= 0 {
		return time.Time{}
	}
	return t.Time
}

func workflowResult(s v1alpha1.WorkflowState) (string, sdktrace.Status) {
	switch s {
	case v1alpha1.WorkflowStateSuccess:
		return "success", sdktrace.Status{Code: codes.Unset}
	case v1alpha1.WorkflowStateTimeout:
		return "timeout", sdktrace.Status{Code: codes.Error, Description: "workflow timed out"}
	default:
		return "failure", sdktrace.Status{Code: codes.Error, Description: "workflow failed"}
	}
}

func countActions(w *v1alpha1.Workflow) int {
	n := 0
	for _, t := range w.Status.Tasks {
		n += len(t.Actions)
	}
	return n
}

// hardwareAttributes describes the machine from the attributes Tinkerbell collected about it.
func hardwareAttributes(hw *v1alpha1.Hardware) []attribute.KeyValue {
	if hw == nil || hw.Status.Attributes == nil {
		return nil
	}
	var attrs []attribute.KeyValue
	add := func(key, val string) {
		if val != "" {
			attrs = append(attrs, attribute.String(key, val))
		}
	}
	for _, a := range []*v1alpha1.Attributes{hw.Status.Attributes.OutOfBand, hw.Status.Attributes.InBand} {
		if a == nil {
			continue
		}
		if a.Product != nil {
			add("tinkerbell.hardware.vendor", a.Product.Vendor)
			add("tinkerbell.hardware.model", a.Product.Model)
		}
		if a.BMC != nil {
			add("tinkerbell.bmc.vendor", a.BMC.Vendor)
			add("tinkerbell.bmc.firmware_version", a.BMC.FirmwareVersion)
		}
		if len(attrs) > 0 {
			break
		}
	}
	return attrs
}

// startReconcileSpan adds what's known about the Workflow before the reconcile changes it.
func startReconcileSpan(span trace.Span, w *v1alpha1.Workflow) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("tinkerbell.workflow.uid", string(w.UID)),
		attribute.String("tinkerbell.workflow.state_before", string(w.Status.State)),
	)
	if w.Spec.HardwareRef != "" {
		span.SetAttributes(
			attribute.String("tinkerbell.hardware.name", w.Spec.HardwareRef),
			attribute.String("tinkerbell.hardware.namespace", w.Namespace),
		)
	}
	if sc, ok := tinkotel.WorkflowRootSpanContext(string(w.UID)); ok {
		span.AddLink(trace.Link{SpanContext: sc})
	}
}

// endReconcileSpan records the reconcile's outcome and ends the span.
// w is nil if the reconcile ended before it could change the Workflow.
func endReconcileSpan(ctx context.Context, span trace.Span, w *v1alpha1.Workflow, result reconcile.Result, err error) {
	defer span.End()
	if !span.IsRecording() {
		return
	}
	if w != nil {
		span.SetAttributes(attribute.String("tinkerbell.workflow.state_after", string(w.Status.State)))
	}
	entries := journal.Journal(ctx)
	msgs := make([]string, 0, len(entries))
	for _, e := range entries {
		msgs = append(msgs, e.Msg)
	}
	span.SetAttributes(
		attribute.StringSlice("tinkerbell.reconcile.journal", msgs),
		attribute.Bool("tinkerbell.reconcile.requeue", result.Requeue || result.RequeueAfter > 0), //nolint:staticcheck // SA1019: Requeue is still used by this controller.
		attribute.Int64("tinkerbell.reconcile.requeue_after_ms", result.RequeueAfter.Milliseconds()),
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
}

func newSpanID() trace.SpanID {
	var id trace.SpanID
	for !id.IsValid() {
		_, _ = rand.Read(id[:])
	}
	return id
}
