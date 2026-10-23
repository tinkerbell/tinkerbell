package otel

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const testUID = "0af76519-16cd-43dd-8448-eb211c80319c"

func TestWorkflowTraceID(t *testing.T) {
	tests := map[string]struct {
		uid    string
		want   string
		wantOK bool
	}{
		"uuid":     {uid: testUID, want: "0af7651916cd43dd8448eb211c80319c", wantOK: true},
		"empty":    {uid: ""},
		"not uuid": {uid: "not-a-uuid"},
		"all zero": {uid: "00000000-0000-0000-0000-000000000000"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := WorkflowTraceID(tt.uid)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.String() != tt.want {
				t.Fatalf("trace ID = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestWorkflowRootSpanContext(t *testing.T) {
	a, ok := WorkflowRootSpanContext(testUID)
	if !ok || !a.IsValid() {
		t.Fatalf("expected a valid span context, got %v", a)
	}
	b, _ := WorkflowRootSpanContext(testUID)
	if !a.Equal(b) {
		t.Fatalf("span context is not stable: %v != %v", a, b)
	}
	if !a.IsSampled() {
		t.Fatal("expected the sampled flag")
	}
	other, _ := WorkflowRootSpanContext("1af76519-16cd-43dd-8448-eb211c80319c")
	if other.SpanID() == a.SpanID() {
		t.Fatal("different UIDs gave the same root span ID")
	}
	if _, ok := WorkflowRootSpanContext("bad"); ok {
		t.Fatal("expected false for a UID that isn't a UUID")
	}
}

func TestWithWorkflowLink(t *testing.T) {
	want, _ := WorkflowRootSpanContext(testUID)
	cfg := trace.NewSpanStartConfig(WithWorkflowLink(testUID))
	if len(cfg.Links()) != 1 || !cfg.Links()[0].SpanContext.Equal(want) {
		t.Fatalf("links = %v, want one link to %v", cfg.Links(), want)
	}
	if cfg := trace.NewSpanStartConfig(WithWorkflowLink("bad")); len(cfg.Links()) != 0 {
		t.Fatalf("expected no links, got %v", cfg.Links())
	}
}

func TestEmitterEmit(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	res := resource.NewSchemaless()
	e := NewEmitter(sdktrace.NewSimpleSpanProcessor(exp), res)
	if !e.Enabled() {
		t.Fatal("expected enabled")
	}
	root, _ := WorkflowRootSpanContext(testUID)
	unsampled := trace.NewSpanContext(trace.SpanContextConfig{TraceID: root.TraceID(), SpanID: trace.SpanID{1}})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	e.Emit("test", []tracetest.SpanStub{
		{Name: "root", SpanContext: root, StartTime: start, EndTime: start.Add(time.Minute)},
		{Name: "child", SpanContext: unsampled, Parent: root, StartTime: start, EndTime: start.Add(time.Second)},
	})

	got := exp.GetSpans()
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2", len(got))
	}
	for _, s := range got {
		if s.Resource != res {
			t.Errorf("%s: resource not set", s.Name)
		}
		if s.InstrumentationScope.Name != "test" {
			t.Errorf("%s: scope = %q, want test", s.Name, s.InstrumentationScope.Name)
		}
		if !s.SpanContext.IsSampled() {
			t.Errorf("%s: not sampled", s.Name)
		}
	}
	if !got[0].StartTime.Equal(start) || got[1].Parent.SpanID() != root.SpanID() {
		t.Fatalf("timestamps or parent not preserved: %+v", got)
	}
}

func TestNilEmitter(t *testing.T) {
	var e *Emitter
	if e.Enabled() {
		t.Fatal("nil emitter should be disabled")
	}
	e.Emit("test", []tracetest.SpanStub{{Name: "x"}})
}

func TestInitSetsDefaultEmitter(t *testing.T) {
	if DefaultEmitter() != nil {
		t.Fatal("expected no default emitter before Init")
	}
	_, shutdown, err := Init(context.Background(), Config{Servicename: "test", Endpoint: "127.0.0.1:1", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if !DefaultEmitter().Enabled() {
		t.Fatal("expected an enabled default emitter after Init")
	}
	shutdown()
	if DefaultEmitter() != nil {
		t.Fatal("expected no default emitter after shutdown")
	}
}
