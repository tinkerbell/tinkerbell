package controller

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLinkToWorkflow(t *testing.T) {
	const uid = "0af76519-16cd-43dd-8448-eb211c80319c"
	controller := true
	tests := map[string]struct {
		owner     metav1.OwnerReference
		wantLinks int
	}{
		"workflow": {owner: metav1.OwnerReference{Kind: "Workflow", Name: "wf", UID: uid, Controller: &controller}, wantLinks: 1},
		"other":    {owner: metav1.OwnerReference{Kind: "Something", Name: "x", UID: uid, Controller: &controller}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			exp := tracetest.NewInMemoryExporter()
			_, span := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)).Tracer("test").Start(context.Background(), "s")
			linkToWorkflow(span, &bmc.Job{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{tt.owner}}})
			span.End()

			links := exp.GetSpans()[0].Links
			if len(links) != tt.wantLinks {
				t.Fatalf("got %d links, want %d", len(links), tt.wantLinks)
			}
			if tt.wantLinks == 1 {
				want, _ := tinkotel.WorkflowRootSpanContext(uid)
				if !links[0].SpanContext.Equal(want) {
					t.Fatalf("link = %v, want %v", links[0].SpanContext, want)
				}
			}
		})
	}
}
