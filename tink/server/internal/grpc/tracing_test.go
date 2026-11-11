package grpc

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnnotateWorkflow(t *testing.T) {
	const uid = "0af76519-16cd-43dd-8448-eb211c80319c"
	exp := tracetest.NewInMemoryExporter()
	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)).Tracer("test").Start(context.Background(), "rpc")
	annotateWorkflow(ctx, &tinkerbell.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "tink", UID: uid},
		Spec:       tinkerbell.WorkflowSpec{HardwareRef: "machine1"},
	})
	span.End()

	got := exp.GetSpans()[0]
	want, _ := tinkotel.WorkflowRootSpanContext(uid)
	if len(got.Links) != 1 || !got.Links[0].SpanContext.Equal(want) {
		t.Fatalf("links = %+v, want the workflow root", got.Links)
	}
	attrs := map[string]string{}
	for _, kv := range got.Attributes {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if attrs["tinkerbell.workflow.uid"] != uid || attrs["tinkerbell.hardware.name"] != "machine1" {
		t.Fatalf("attributes = %v", attrs)
	}
}
