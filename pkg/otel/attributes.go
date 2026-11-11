package otel

import (
	"context"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AnnotateHardware adds a Hardware's name and namespace to the span in ctx.
// tinkerbell.hardware.name is the attribute for finding every trace about one machine.
func AnnotateHardware(ctx context.Context, hw *tinkerbell.Hardware) {
	if hw == nil {
		return
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("tinkerbell.hardware.name", hw.Name),
		attribute.String("tinkerbell.hardware.namespace", hw.Namespace),
	)
}
