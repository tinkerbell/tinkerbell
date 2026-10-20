package otel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// WorkflowTraceID returns the trace ID of a Workflow trace: the Workflow's
// UID with the dashes removed. It returns false if the UID isn't a UUID.
func WorkflowTraceID(uid string) (trace.TraceID, bool) {
	b, err := hex.DecodeString(strings.ReplaceAll(uid, "-", ""))
	if err != nil || len(b) != len(trace.TraceID{}) {
		return trace.TraceID{}, false
	}
	tid := trace.TraceID(b)
	return tid, tid.IsValid()
}

// WorkflowRootSpanContext returns the span context of a Workflow trace's root
// span. Any component that knows the Workflow UID can use it as a link target.
func WorkflowRootSpanContext(uid string) (trace.SpanContext, bool) {
	tid, ok := WorkflowTraceID(uid)
	if !ok {
		return trace.SpanContext{}, false
	}
	sum := sha256.Sum256([]byte("tinkerbell/workflow/" + uid))
	sid := trace.SpanID(sum[:8])
	if !sid.IsValid() {
		sid = trace.SpanID(sum[8:16])
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}), true
}

// WithWorkflowLink returns a span start option that links the span to the root
// of the Workflow trace for uid. It adds nothing if uid isn't a UUID.
func WithWorkflowLink(uid string) trace.SpanStartOption {
	sc, ok := WorkflowRootSpanContext(uid)
	if !ok {
		return trace.WithLinks()
	}
	return trace.WithLinks(trace.Link{SpanContext: sc})
}

// SpanContextFromTraceparent parses a W3C traceparent value.
func SpanContextFromTraceparent(traceparent string) (trace.SpanContext, bool) {
	if traceparent == "" {
		return trace.SpanContext{}, false
	}
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": traceparent})
	sc := trace.SpanContextFromContext(ctx)
	return sc, sc.IsValid()
}

// Emitter sends spans that were built after the fact, such as a Workflow trace,
// to the same span processor and resource as the live spans.
// A nil Emitter is disabled.
type Emitter struct {
	processor sdktrace.SpanProcessor
	resource  *resource.Resource
}

// NewEmitter returns an Emitter that sends spans to processor with resource res.
func NewEmitter(processor sdktrace.SpanProcessor, res *resource.Resource) *Emitter {
	return &Emitter{processor: processor, resource: res}
}

// Enabled reports whether spans sent to e go anywhere.
func (e *Emitter) Enabled() bool {
	return e != nil && e.processor != nil
}

// Emit hands finished spans to the span processor. It doesn't block.
// The resource, instrumentation scope, and sampled flag are set on each span.
func (e *Emitter) Emit(scope string, spans []tracetest.SpanStub) {
	if !e.Enabled() {
		return
	}
	for _, s := range spans {
		s.Resource = e.resource
		s.InstrumentationScope = instrumentation.Scope{Name: scope}
		s.SpanContext = s.SpanContext.WithTraceFlags(s.SpanContext.TraceFlags() | trace.FlagsSampled)
		e.processor.OnEnd(s.Snapshot())
	}
}

var defaultEmitter atomic.Pointer[Emitter]

func setDefaultEmitter(e *Emitter) { defaultEmitter.Store(e) }

// DefaultEmitter returns the Emitter set up by Init, or nil when tracing is off.
func DefaultEmitter() *Emitter { return defaultEmitter.Load() }
