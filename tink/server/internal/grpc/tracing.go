package grpc

import (
	"context"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// annotateSpan adds attributes to the current request span.
func annotateSpan(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// annotateWorkflow describes the Workflow on the current request span and links the span to the Workflow trace.
func annotateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("tinkerbell.workflow.uid", string(wf.UID)),
		attribute.String("tinkerbell.workflow.name", wf.Name),
		attribute.String("tinkerbell.workflow.namespace", wf.Namespace),
		attribute.String("tinkerbell.workflow.state", string(wf.Status.State)),
	)
	if wf.Spec.HardwareRef != "" {
		span.SetAttributes(
			attribute.String("tinkerbell.hardware.name", wf.Spec.HardwareRef),
			attribute.String("tinkerbell.hardware.namespace", wf.Namespace),
		)
	}
	if sc, ok := tinkotel.WorkflowRootSpanContext(string(wf.UID)); ok {
		span.AddLink(trace.Link{SpanContext: sc})
	}
}
