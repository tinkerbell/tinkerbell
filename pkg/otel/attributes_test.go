package otel

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnnotateHardware(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)).Tracer("test").Start(context.Background(), "s")
	AnnotateHardware(ctx, nil)
	AnnotateHardware(ctx, &tinkerbell.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink"}})
	span.End()

	attrs := map[string]string{}
	for _, kv := range exp.GetSpans()[0].Attributes {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	if attrs["tinkerbell.hardware.name"] != "machine1" || attrs["tinkerbell.hardware.namespace"] != "tink" {
		t.Fatalf("attributes = %v", attrs)
	}
}
