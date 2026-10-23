package controller

import (
	"context"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const tracerName = "github.com/tinkerbell/tinkerbell/rufio"

func startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// endSpan records err, if any, and ends span.
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// linkToWorkflow links span to the Workflow trace when a Workflow controls obj.
func linkToWorkflow(span trace.Span, obj metav1.Object) {
	o := metav1.GetControllerOf(obj)
	if o == nil || o.Kind != "Workflow" {
		return
	}
	span.SetAttributes(
		attribute.String("tinkerbell.workflow.uid", string(o.UID)),
		attribute.String("tinkerbell.workflow.name", o.Name),
		attribute.String("tinkerbell.workflow.namespace", obj.GetNamespace()),
	)
	if sc, ok := tinkotel.WorkflowRootSpanContext(string(o.UID)); ok {
		span.AddLink(trace.Link{SpanContext: sc})
	}
}

// startBMCCall starts a client span for a call to a BMC.
func startBMCCall(ctx context.Context, name string, a bmc.Action) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("tinkerbell.bmc.action", actionName(a))),
	)
}

// endBMCCall records which bmclib providers were tried and which one worked, then ends span.
func endBMCCall(span trace.Span, c *bmclib.Client, err error) {
	if c != nil {
		md := c.GetMetadata()
		span.SetAttributes(
			attribute.StringSlice("tinkerbell.bmc.providers_attempted", md.ProvidersAttempted),
			attribute.String("tinkerbell.bmc.provider", md.SuccessfulProvider),
		)
	}
	endSpan(span, err)
}

func actionName(a bmc.Action) string {
	switch {
	case a.PowerAction != nil:
		return "power_" + string(*a.PowerAction)
	case a.BootDevice != nil:
		return "boot_device"
	case a.OneTimeBootDeviceAction != nil: //nolint:staticcheck // oneTimeBootDeviceAction is deprecated but still supported.
		return "one_time_boot_device"
	case a.VirtualMediaAction != nil:
		return "virtual_media"
	case a.NetworkBootConfig != nil:
		return "network_boot_config"
	default:
		return "none"
	}
}
