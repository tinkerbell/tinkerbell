# Tracing

Tinkerbell sends OpenTelemetry traces over OTLP/gRPC. For the design and the reasoning
behind it, see [designs/OTEL_TRACING.md](designs/OTEL_TRACING.md).

## Turn it on

Set the collector endpoint. Tracing is off when it's empty.

| Flag | Env var | Helm value |
|---|---|---|
| `--otel-endpoint` | `TINKERBELL_OTEL_ENDPOINT` | `deployment.envs.globals.otelEndpoint` |
| `--otel-insecure` | `TINKERBELL_OTEL_INSECURE` | `deployment.envs.globals.otelInsecure` |

TLS with the system roots is the default. Everything else uses the standard
[OpenTelemetry environment variables](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/),
for example:

```yaml
deployment:
  envs:
    globals:
      otelEndpoint: "otel-collector.observability:4317"
  additionalEnvs:
    - name: OTEL_EXPORTER_OTLP_HEADERS
      value: "x-honeycomb-team=<api key>"
    - name: OTEL_TRACES_SAMPLER
      value: "parentbased_traceidratio"
    - name: OTEL_TRACES_SAMPLER_ARG
      value: "0.25"
```

The sampler applies to request traces only. Workflow traces are always sent.

## What you get

**One trace per Workflow,** sent once, when the Workflow reaches `SUCCESS`, `FAILED`,
or `TIMEOUT`. It has:
- a `tinkerbell.workflow` root span, from the first BMC Job or Action that ran to the
  Workflow's end
- one `tinkerbell.bmc.job` span per finished BMC Job
- one `tinkerbell.action` span per finished Action.

Deleted Workflows and Workflows that never finish have no Workflow trace. Workflows
that ended more than an hour before the controller saw them are skipped. After the
trace is sent, the Workflow has a `TraceEmitted` condition.

**A short trace for each piece of work:** every controller and rufio reconcile, BMC
call, DHCP packet, iPXE, ISO, or bootstrap request, metadata request, and tink-server
call.

## Find traces

The Workflow trace ID is the Workflow's UID without dashes:

```bash
kubectl get workflow -n tink my-workflow -o jsonpath='{.metadata.uid}' | tr -d -
```

To find everything about a machine, in flight or not, query by attribute:
- `tinkerbell.hardware.name` is on every span that knows the machine.
- `tinkerbell.workflow.uid` is on every span that knows the Workflow.

Controller reconciles, rufio Job reconciles, and tink-server calls also link to the
Workflow trace's root span.

## Linking from your own trace

To connect a Workflow trace to the system that created the Workflow, set a W3C
traceparent on the Workflow when you create it:

```yaml
metadata:
  annotations:
    tinkerbell.org/traceparent: "00-<trace id>-<span id>-01"
```

The Workflow trace's root span gets a link to it.

## Trust

smee, tootles, and tink-server don't trust trace context from clients. Incoming
`traceparent` headers become a link on a new trace, not a parent.
