# OpenTelemetry Tracing

Status: implemented. Operator guide: [TRACING.md](../TRACING.md).

## Summary

When a Workflow ends, the Workflow controller builds one small trace for it from data
already saved in Kubernetes and sends it. The trace has a root span from start to
finish, one span per BMC Job, and one span per Action. Its trace ID is the Workflow's
UID.

Everything else stays the way OpenTelemetry already works: each reconcile, DHCP
packet, iPXE fetch, metadata request, and tink-server call is its own short trace.
These spans carry the Hardware name, and the Workflow UID when they know it, so you
can find every trace for a machine or a Workflow by querying one attribute. When the
Workflow UID is known, the span also links to the Workflow trace.

There's one rule for closing a trace: **a Workflow trace is sent once, when the
Workflow reaches `SUCCESS`, `FAILED`, or `TIMEOUT`.** Nothing is sent before that,
so no trace is ever left half-finished. A Workflow that's deleted or never finishes
has no Workflow trace. Its request traces still exist.

The design adds no API fields, CRD changes, annotations, or flags. The implementation
is about 515 lines of non-test Go in new files, not counting blank lines and
comments, plus small edits at existing call sites. That's more than the 300–400 lines
first estimated; most of the difference is the span builder in the Workflow
controller.

Instrumentation of the Tink Agent is out of scope.

## Goals

- Answer "what happened to this provisioning run, and where did the time go?" from
  one trace, found by the Workflow's UID.
- Answer "what has this machine been doing?" by querying one attribute across all
  traces.
- Put the facts on each span as attributes (Workflow, Hardware, vendor, model, BMC,
  Template, Action, image, result) so you can ask new questions without new code.
- Survive controller restarts, more than one controller replica, and tink-server
  writing the terminal state.
- Write as little code as possible.
- Cost nothing when tracing is off, and never block, slow, or fail provisioning when
  it's on.

## Non-goals

- Tink Agent instrumentation: spans inside the booted OS, and sending trace context
  to the Agent. See [Future: the Agent](#future-the-agent).
- A Workflow trace for Workflows that are deleted or never finish. See
  [What this design leaves out](#what-this-design-leaves-out).
- Guaranteed delivery. Workflow traces get the same best-effort delivery as every
  other span.
- OTLP log export and metrics.

## Design principles

1. **Code is a liability.** Each line has to be read, tested, and kept working long
   after it's written ([Doctorow, 2026](https://pluralistic.net/2026/01/06/1000x-liability/)).
   Use what Kubernetes, the Workflow status, and the OTel SDK already provide. Add a
   mechanism only if the value is clear and nothing that already exists does the
   job.
2. **Tracing reports. It doesn't decide.** The Workflow's own state machine decides
   when a Workflow is done, and tracing follows it. If hung Workflows should end,
   that's a Workflow timeout feature. When it exists, those Workflows get traces too.
3. **A span never outlives the call that ends it.** Live spans start and end in one
   function. The Workflow trace is built after the Workflow ends, from saved
   timestamps.
4. **Don't fabricate timing.** Spans are built only from timestamps that were
   actually recorded. Missing data is left out, not estimated.
5. **Caused-by is a link. Called-by is a parent.** A reconcile or a BMC call is
   caused by a Workflow, not called by it, so it gets its own trace and a link.
6. **Wide spans, few spans.** Add an attribute before adding a span.
7. **Tracing never changes provisioning.** No finalizers, no blocking exports, no
   extra requeues. Failures are logged and dropped.

## Background

### The `otel-plumb` branch

The `otel-plumb` branch (jacobweinstock/tinkerbell, May 2026, not merged) put every
component into one trace per Workflow. Its problems:

- **Root spans with no meaning.** The root span and the phase spans were ended the
  moment they were created, so they had no duration and no result.
- **Traces with no end.** Nothing marked a trace as finished, so spans kept arriving
  for as long as anything touched the machine.
- **A span on every reconcile.** These were added to the Workflow trace, including
  every requeue, so the trace was mostly idle gaps.
- **A stale Hardware annotation.** The traceparent annotation on Hardware was written
  on every reconcile and never removed. Every DHCP and metadata request for that
  machine after the Workflow ended landed in the old trace.
- **Split traces.** The trace ID was generated in memory. If the status patch that
  saved it failed, the next reconcile created a second trace.

What to keep from it:

- W3C `traceparent`.
- Wide DHCP and BMC attributes.
- Spans around BMC calls.
- The existing smee chain from a DHCP reply to the iPXE binary fetch.

### The first draft of this design

The first draft fixed those problems with a lot of machinery:

- a new `status.trace` API struct with a state history
- trace context annotations on Hardware and BMC Jobs
- a delete-event handler and a 24-hour maximum trace age
- synchronous export with retries
- per-component tracer providers
- three new flags.

Two independent reviews (GPT-6.1 Sol and Gemini 4 Argon, October 2026) checked it
against the code and found that most of its bugs came from that machinery:

- **Frozen BMC polling.** Adding `RequeueAfter` for the maximum age would override the
  `Requeue: true` the controller uses to poll BMC Jobs, which could stall provisioning
  for up to 24 hours.
- **Write conflicts.** Writing state history while a Workflow runs would cause 409
  conflicts with tink-server's status updates.
- **Blocked reconciles.** A synchronous export would block the controller's single
  reconcile worker while the collector is down.
- **Side effects on Hardware.** Hardware annotation writes would trigger Hardware
  template re-renders and conflict with the controller's own `allowPXE` patches. A
  crash at the wrong moment would leave stale annotations behind.
- **An older tink-server erasing the new field.** During a rolling upgrade, an older
  tink-server's full status update would silently remove the new `status.trace`
  field.

This version removes that machinery instead of repairing it. See
[What this design leaves out](#what-this-design-leaves-out) for what was given up.

### What other projects learned

| Source | Lesson |
|---|---|
| Tekton ([TEP-0124](https://github.com/tektoncd/community/blob/main/teps/0124-distributed-tracing-for-tasks-and-pipelines.md), [#9699](https://github.com/tektoncd/pipeline/pull/9699), [#10866](https://github.com/tektoncd/pipeline/issues/10866)) | Shipped a root span that ended at once (fixed in #9699). An open proposal (#10866) says reusing one trace across every reconcile makes "the root duration reflect the CR's lifetime and requeue gaps, not build work." |
| Argo Workflows ([tracing docs](https://github.com/argoproj/argo-workflows/blob/main/docs/tracing.md), [#16899](https://github.com/argoproj/argo-workflows/issues/16899)) | Keeps live spans in an in-memory map: "Spans that start in one workflow controller and end in a different workflow controller ... will be incomplete." #16899 shows how fragile that map is: a double `EndNode` call produced about 1,200 errors an hour. |
| GitHub Actions ([githubreceiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/githubreceiver/README.md), [otel-cicd-action](https://github.com/dash0hq/otel-cicd-action)) | Builds the whole trace after the run completes, from saved timing data, with trace and span IDs derived from run and job IDs. |
| OTel Go maintainer on Argo's restart problem ([semantic-conventions#1648](https://github.com/open-telemetry/semantic-conventions/issues/1648#issuecomment-2532369389)) | "As long as you can store a trace ID, span ID, and start time, you can probably just generate the entire span when it ends." |
| Kubernetes [KEP-647](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/647-apiserver-tracing/README.md) | Controller tracing is a non-goal. The API server only trusts incoming `traceparent` from privileged callers. |
| Kubernetes [KEP-5915](https://github.com/kubernetes/enhancements/issues/5915) (proposed, 2026; [design PR](https://github.com/kubernetes/enhancements/pull/5916) open) | For async work, a reconciler starts a new root span with a span link, "without implying a synchronous parent-child structure." |
| Cluster API ([#3760](https://github.com/kubernetes-sigs/cluster-api/issues/3760)) | Annotation-based lifecycle tracing was proposed in 2020 and never built. The discussion settled on one span per reconcile plus spans around network calls. |
| Charity Majors ([O11y 2.0](https://www.honeycomb.io/blog/time-to-version-observability-signs-point-to-yes), [structured events](https://charity.wtf/2019/02/05/logs-vs-structured-events/)) | One arbitrarily wide event per unit of work per service, with high-cardinality fields. A trace is a view over those events. You should be able to ask new questions without shipping new code. |
| Honeycomb, "Span or attribute" ([2026](https://www.honeycomb.io/blog/span-or-attribute-opentelemetry-custom-instrumentation)) | "TL;DR: Attribute." Start a new span only for an incoming request, a network boundary, an async boundary, or a performance concern. |
| Honeycomb, "Exotic Trace Shapes" ([2024](https://www.honeycomb.io/blog/exotic-trace-shapes)), "Tracing is for async, too" ([2022](https://www.honeycomb.io/blog/tracing-is-for-async-too)) | For long or async work: a trace per unit of work, links to what started it, backdated timestamps. |
| Honeycomb docs ([missing root](https://docs.honeycomb.io/troubleshoot/common-issues/data-in-honeycomb), [Refinery](https://docs.honeycomb.io/manage-data-volume/sample/honeycomb-refinery/diagnose)) | A missing root span drops the trace from root-span queries. Traces whose spans arrive over a long time are a known tail-sampling problem. |

## Trace model

### The Workflow trace

```text
tinkerbell.workflow          root      start → end
├── tinkerbell.bmc.job       child     one per finished BMC Job (netboot, iso-mount, customboot, ...)
└── tinkerbell.action        child     one per Action that has start and stop times
```

- **Root.** Starts at the first thing that actually ran: the earliest BMC Job
  `startTime` or Action `executionStart`. This follows how the Workflow runs:
  - **With a `bootMode`:** Tinkerbell drives the boot, so the root starts when the
    first BMC Job starts. The BMC work and the machine's boot are part of the
    provisioning time.
  - **Without a `bootMode`:** Tinkerbell can't know when someone powers the machine
    on, so the root starts at the first Action. Days spent waiting in `PENDING`
    for a machine to show up don't count as provisioning time.
  - **With `spec.disabled: true`:** nothing runs until the Workflow is enabled, so
    nothing counts before then.
  - **If nothing ran:** for example, a failure before any Job was created. The root
    starts at its end time and has zero length. It still carries the result.

  The root ends at the latest timestamp recorded in the Workflow status: Action stop
  times, condition times except `TraceEmitted`, and `globalExecutionStop` for a global
  timeout. If there is none, it ends at the time the trace is built.

  The end is therefore set by the Workflow's own data, not by when the controller
  happens to notice. It stays correct after controller downtime, and building the
  trace twice gives the same result.
- **BMC Job.** For each entry in `status.bootOptions.jobs`, the controller gets the
  rufio Job, checking that its UID matches. If the Job has both `startTime` and
  `completionTime`, it gets a span. rufio sets `completionTime` only on success, so a
  failed Job gets no span. It appears on the root as
  `tinkerbell.workflow.failed_bmc_job`, and the details are in rufio's own traces.
- **Action.** A span if both `executionStart` and `executionStop` are set and not
  zero. The Agent reports `RUNNING` before it records a start time
  ([agent.go](../../../tink/agent/agent.go)), so an Action still running when the
  Workflow ended has no usable start. It gets no span. Instead, the root names it in
  `tinkerbell.workflow.current_action`.
- **No other spans.** States and Tasks are not separate spans:
  - The state timeline is visible as the gaps between Job spans and Action spans.
  - Durations are on the root (see [Attributes](#attributes)).
  - Task name and ID are attributes on each Action span.
- **Size.** About `1 + jobs + actions` spans, usually under 30.

### Request traces

These are live, ordinary spans. Each is its own trace.

| Component | Span | Correlation |
|---|---|---|
| Workflow controller | One per reconcile | Workflow and Hardware attributes, and a link to the Workflow root |
| rufio | One per Job and Task reconcile, plus a client span per `bmclib` call | Job reconciles: link to the Workflow root through the Job's controller owner reference. Task reconciles: the Job name, from the Task's owner reference. |
| tink-server | `otelgrpc` server span per call | Workflow, Agent, Task, and Action attributes, and a link to the Workflow root |
| smee | One per DHCP packet; HTTP and TFTP spans for scripts and binaries | Hardware attributes. The binary fetch continues the DHCP trace through the file name, as today. |
| tootles | `otelhttp` span per request | Hardware attributes |

smee and tootles look up Hardware, not Workflows, so their spans carry the Hardware
name and MAC address but no Workflow link. **The join key across every trace is
`tinkerbell.hardware.name`.** `tinkerbell.workflow.uid` is set wherever the Workflow
is known.

## Identity

- **Trace ID:** the Workflow's `.metadata.uid` with the dashes removed. Kubernetes
  UIDs are 16 random bytes (UUIDv4 from `google/uuid`), so they're valid, unique
  trace IDs.

  ```bash
  kubectl get workflow -n tink my-workflow -o jsonpath='{.metadata.uid}' | tr -d -
  ```

- **Root span ID:** `sha256("tinkerbell/workflow/" + uid)[0:8]`.
- **Child span IDs:** random. Nothing outside the Workflow trace refers to them, so
  they don't need to be deterministic.
- **Links:** any component that knows the Workflow UID can build the link target
  (trace ID and root span ID) without a lookup or a carrier. A link to a trace that
  was never sent is harmless.
- **`tinkerbell.workflow.uid`:** always the Kubernetes form, with dashes.

The Workflow trace is never sampled with the SDK's `TraceIDRatioBased` sampler. That
sampler reads `TraceID[8:16] >> 1`, and the fixed UUID variant bits put every UID in
the `[0.5, 0.75)` range, so a ratio of 0.5 or below would drop every Workflow trace.
See [Sampling](#sampling).

### Incoming trace context

A Workflow's creator (CAPT, a CLI, a CI system) can set the annotation
`tinkerbell.org/traceparent` on the Workflow. When the controller builds the
Workflow trace, the root span gets a link to it. The controller only reads it once,
writes nothing, and never uses it as a parent. If KEP-5915 ships a standard key, the
controller should read that too.

## Sending the Workflow trace

The controller sends the trace from one hook in the reconcile branch for `SUCCESS`,
`FAILED`, and `TIMEOUT`, when the Workflow doesn't have a `TraceEmitted` condition.
That covers terminal states tink-server writes and the ones the controller writes
itself: writing a terminal state patches the status, and the patch triggers the
reconcile that runs the hook.

1. Add `{Type: TraceEmitted, Status: True}` to the status and patch it.
2. The patch uses the existing optimistic lock in `mergePatchStatus`. Only the
   reconcile whose patch succeeds continues to step 3. That's how duplicates are
   prevented, whether from a stale cache, a second replica, or a retry.
3. Build the spans (see [Trace model](#trace-model)) and hand them to the SDK's batch
   span processor. This doesn't block. The batch processor exports them with the
   exporter's normal retry.

**Skipping old Workflows.** If the root's end time is more than an hour old when the
hook first sees the Workflow, the hook does nothing: no condition and no trace. When
tracing is first turned on, this stops the controller from writing to, and sending a
trace for, every Workflow that finished before. It also skips Workflows that
finished while the controller was down for more than an hour.

**Delivery is at most once.** A crash between the patch and the batch processor's
export loses the trace. So does a full export queue, or a collector that stays down
past the exporter's retries. The same is true of every other span Tinkerbell sends.

### Why a Condition

- It needs no API or CRD change. `type` is a free-form string.
- Every Tinkerbell version already reads and writes `conditions`, so an older
  tink-server's full status update keeps it during a rolling upgrade.
- It's visible with `kubectl`.
- It's only written when tracing is on, and only once per Workflow, when it has
  already ended.

### Building spans

`Init` creates a `pkg/otel.Emitter` that holds the batch span processor and resource.
`DefaultEmitter()` returns it, or nil when tracing is off:

```go
// Emit hands finished spans to the span processor. It doesn't block.
func (e *Emitter) Emit(scope string, spans []tracetest.SpanStub)
```

`Emit` sets the Resource, instrumentation scope, and sampled flag on each stub, calls
`Snapshot()`, and passes the result to the processor's `OnEnd`. A nil `Emitter` is
disabled, and tests pass their own `Emitter` to the reconciler.

`tracetest.SpanStub` is an exported, stable struct for building a `ReadOnlySpan` with
your own IDs and timestamps. The alternative is a context-aware `IDGenerator` on the
global provider plus backdated `tracer.Start` calls. That would put a special case in
every span's creation path and run the Workflow trace through the sampler, which is
the UUID problem above.

## Component changes

### `pkg/otel`

- Add:
  - `WorkflowTraceID(uid)`
  - `WorkflowRootSpanContext(uid)` and `WithWorkflowLink(uid)` for link targets
  - `SpanContextFromTraceparent`
  - `Emitter` and `DefaultEmitter()`
  - `AnnotateHardware(ctx, hw)`.
- In `Init`:
  - Remove the gRPC retry policy that retries 1,000 times every 10 ms. The exporter
    already retries with backoff.
  - Stop passing explicit TLS credentials. Explicit options override
    `OTEL_EXPORTER_OTLP_CERTIFICATE` and related variables, and the default is
    already TLS.
  - Don't set a sampler, so `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` work.
  - Update `semconv` from `v1.4.0`.
- One tracer provider for the process, with `service.name=tinkerbell`. Components are
  told apart by instrumentation scope, for example
  `github.com/tinkerbell/tinkerbell/smee`.

### Workflow controller

- Add the [send hook](#sending-the-workflow-trace).
- One span per reconcile, with:
  - attributes: Workflow, Hardware, the state before and after, the requeue, and any
    error
  - the existing `journal` messages as a string list attribute
  - a link to the Workflow root.

### rufio

- One span per Job and Task reconcile.
- A client span around running a Task's action and around checking its status, with
  the action, the `bmclib` providers tried, the provider that worked, and the result.
- Jobs: a link to the Workflow root, built from the controller owner reference when
  its kind is `Workflow`.
- Tasks: `tinkerbell.bmc.job.name` from their owner reference. No extra reads.

### tink-server

- Workflow, Agent, Task, and Action attributes on the `otelgrpc` span.
- A link to the Workflow root.

### smee and tootles

- Call `AnnotateHardware` on the request span right after each Hardware lookup. It
  sets `tinkerbell.hardware.name` and `.namespace`, and does nothing if no Hardware
  was found.
- The binary handler has no Hardware and doesn't need it: its trace continues from
  the DHCP span.

### Trust boundary

smee, tootles, and tink-server serve the provisioning network, which isn't
authenticated. Their incoming trace context should become a **link** on a new
root, not a parent, so nobody on that network can add spans to someone else's trace
or force sampling:
- tink-server: `otelgrpc.WithPublicEndpoint()`
- smee and tootles: `otelhttp.WithPublicEndpointFn`, through the shared HTTP
  middleware.

smee's own DHCP-to-binary chain isn't affected, because smee reads the file name
itself.

## Attributes

All names use the `tinkerbell.*` prefix. Result values match the CI/CD semantic
conventions' `cicd.pipeline.result`: `success`, `failure`, `timeout`.

| Attribute | On |
|---|---|
| `tinkerbell.workflow.uid`, `.name`, `.namespace` | Every span that knows the Workflow |
| `tinkerbell.hardware.name`, `.namespace` | Every span that knows the Hardware |
| `tinkerbell.workflow.template`, `.boot_mode`, `.global_timeout` | Root |
| `tinkerbell.workflow.result` | Root |
| `tinkerbell.workflow.current_action`, `.current_action_state`, `.failed_bmc_job` | Root, when set |
| `tinkerbell.workflow.action.count` | Root |
| `tinkerbell.workflow.before_actions_ms`, `.actions_ms`, `.after_actions_ms` | Root. Computed from the root's start, the first Action's start, the last Action's stop, and the root's end. Left out if an input is missing. |
| `tinkerbell.hardware.vendor`, `.model`, `tinkerbell.bmc.vendor`, `.firmware_version` | Root, from Hardware status attributes when present |
| `tinkerbell.workflow.state_before`, `.state_after`, `tinkerbell.reconcile.journal`, `.requeue`, `.requeue_after_ms` | Controller reconcile |
| `tinkerbell.workflow.state` | tink-server |
| `tinkerbell.agent.id` | Action, tink-server |
| `tinkerbell.action.name`, `.id`, `.image`, `.state`, `.task.name`, `.task.id` | Action. tink-server sets the IDs, name, and state. |
| `tinkerbell.bmc.job.name`, `.job.namespace`, `.machine.name` | rufio Job reconcile |
| `tinkerbell.bmc.task.name`, `.task.namespace`, `.host`, `.job.name` | rufio Task reconcile |
| `tinkerbell.bmc.action`, `.provider`, `.providers_attempted` | rufio BMC calls |

Action times come from the Agent's clock. Everything else comes from the
controller's clock. The duration attributes that mix the two (`before_actions_ms`,
`after_actions_ms`) can be off by the clock skew. That's accepted: it's visible, and
correcting it would need data nobody records.

The Agent reports `"action completed"` as the message for every Action, including
failures ([agent.go](../../../tink/agent/agent.go)), so there's no failure message to
record. The root names the failed Action and its state.

## Sampling

- **Workflow traces are always sent** when tracing is on. They're about `1 + jobs +
  actions` spans per provisioning run, which is too few to be worth sampling.
- **Request traces** use the SDK's standard environment variables
  (`OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`). There are no Tinkerbell flags
  for this.
- **Tail sampling works.** A Workflow trace reaches the collector as one batch, so the
  collector's `tail_sampling` processor and Refinery see the whole trace within their
  normal wait.

## Configuration

No new flags. `--otel-endpoint` and `--otel-insecure` stay as they are. Anything else
uses the standard `OTEL_*` environment variables: headers for vendor API keys,
certificates, compression, and the sampler. They can be set through the Helm chart's
existing `deployment.additionalEnvs`; document them there.

When `--otel-endpoint` is empty:
- no condition is written
- no spans are built
- the hook returns right away.

## Failure modes

| Failure | Result |
|---|---|
| Controller restarts mid-Workflow | Nothing to recover. The trace is built from the status when the Workflow ends. |
| Controller down when the Workflow ends | Sent on restart with the correct end time, if it ended less than an hour before. |
| Two replicas or a stale cache | The optimistic lock on the condition patch lets only one reconcile send. |
| Crash after the patch but before export | That Workflow trace is lost. |
| Collector down | The batch processor retries, then drops. Provisioning isn't affected. |
| Workflow deleted before it ends | No Workflow trace. Request traces remain. |
| Workflow never ends (hung `PENDING`, template error in `""`) | No Workflow trace. Request traces and reconcile spans show what's happening. |
| Workflow created with `spec.disabled: true` | Nothing happens while it's disabled. No trace is open and nothing times out. Once it's enabled, the root starts at the first BMC Job or Action. |
| Workflow without a `bootMode` waits a long time for its machine | The wait isn't in the trace. The root starts at the first Action. |
| Action running when the Workflow times out | No Action span. The root names it in `current_action`. |
| Failed BMC Job | No Job span. Named on the root. Details in rufio's traces. |
| Agent clock skew | Action spans are shifted. Duration attributes that mix clocks are off by the skew. |
| Rolling upgrade or downgrade | No new fields. Older versions keep the condition. |

## What this design leaves out

| Left out | Value lost | Why it's acceptable |
|---|---|---|
| Traces for deleted or never-ending Workflows | No root span for Workflows that were cancelled or hung | Request traces exist, `kubectl` shows the state, and "should this Workflow end?" is the Workflow's decision (principle 2) |
| A span per Workflow state | No `PREPARING`/`PENDING`/`RUNNING`/`POST` bars | The gaps between Job and Action spans show it. The root has the durations. There's no reliable recorded timestamp for every state change. |
| Task spans | No per-Task grouping in the waterfall | Task name and ID are on every Action span |
| Spans for failed BMC Jobs, and BMC detail in the Workflow trace | Failed Jobs have no bar | rufio sets no failure time. rufio's traces have the detail. |
| Workflow links from smee and tootles | No one-click jump from a DHCP or metadata span to the Workflow trace | Query by `tinkerbell.hardware.name`. Adding a link would mean an annotation on Hardware, or a Workflow index in smee and tootles. |
| Guaranteed delivery | A rare crash or collector outage can lose a Workflow trace | The same delivery as every other span, without an outbox or retry state |
| Outcome-aware or ratio sampling of Workflow traces | Can't sample successes while keeping failures | Workflow traces are tiny. Request sampling uses the standard environment variables. |
| Per-component `service.name` | One `tinkerbell` service in service maps | Instrumentation scope tells components apart, with one provider and no shared-exporter shutdown problems |

Each of these can be added later, on its own, if real use shows it's worth the code.

## Alternatives considered

- **The first draft of this design.** It had state history in a new status struct,
  Hardware and Job annotations, a delete handler, a maximum trace age, synchronous
  export, and new flags. That's more code and more risk for the cases this version
  leaves out. See [The first draft of this design](#the-first-draft-of-this-design).
- **A live root span in memory (Argo).** It doesn't survive a restart or a leader
  change, and the Workflow's end happens in other processes.
- **A zero-length root that later spans use as their parent (`otel-plumb`, early
  Tekton).** It has no duration or result, and the trace never ends.
- **Every component as children in one trace (`otel-plumb`).** The trace grows
  without bound, and tail sampling becomes impossible.
- **Sending Workflow spans as each piece ends, root last.** This gives live progress,
  but the spans of one trace would arrive hours apart. That breaks tail sampling and
  is unreliable in some backends. Request traces already show live progress.
- **Ordinary SDK spans with random IDs.** Slightly less code, but there's no stable
  trace ID to look up and nothing to link to before the trace exists.
- **Starting the root when the Workflow is created or its template is rendered.**
  For a Workflow without a `bootMode`, that counts the time spent waiting for someone
  to boot the machine, which can be days. That's idle time presented as work.
- **Starting the root at `RUNNING`.**
  - It drops the BMC work and the machine's boot when Tinkerbell drives the boot,
    which is often most of the time and most of the failures.
  - Nothing records when `RUNNING` began. The first Action's start time is only
    reported when that Action finishes.
- **Choosing the start based on `bootMode`.** Starting at the first BMC Job or
  Action gives the same result with no branch on `bootMode`.

## Future: the Agent

An Agent that knows the Workflow UID can link its own spans to the Workflow root
without any carrier on the kernel command line. Agent spans must not reach the
collector without authentication: KEP-647 trusts incoming trace context only from
privileged callers for the same reason.

A small Agent fix outside this design would make running Actions traceable: set
`ExecutionStart` before reporting `RUNNING`, and report the real error message on
failure.

## Rollout

1. `pkg/otel`: IDs, `EmitSpans`, and the `Init` fixes.
2. Workflow controller: the send hook and reconcile spans.
3. rufio, tink-server, smee, tootles: attributes, links, and the trust boundary.
4. User docs in `docs/technical/`.

Each step works on its own.

## Testing

- **Unit:** the trace ID and root span ID, and building spans from Workflow status
  fixtures:
  - success, failure, global timeout, and Agent-reported timeout
  - missing and zero timestamps
  - failed Jobs
  - an Action still running.
- **Controller:** use a fake client and `tracetest.InMemoryExporter` to check:
  - the condition is written once
  - the trace is sent only after a successful patch
  - a patch conflict sends nothing
  - old Workflows are skipped
  - nothing happens with tracing off.
- **End to end:** run the playground with an OTel Collector and Jaeger or Tempo, and
  provision to success, to failure, and to a global timeout.

## Open questions

1. Is one hour the right cutoff for skipping old Workflows?
2. Should `tinkerbell.org/traceparent` also accept KEP-5915's key now?
3. Should provisioning add a timeout for `PENDING`? It would give hung Workflows a
   terminal state, and so a trace. It's a provisioning change, not a tracing one.

## References

- `otel-plumb` branch: <https://github.com/jacobweinstock/tinkerbell/tree/otel-plumb>
- Cory Doctorow, "Code is a liability (not an asset)":
  <https://pluralistic.net/2026/01/06/1000x-liability/>
- Charity Majors, "It's Time to Version Observability":
  <https://www.honeycomb.io/blog/time-to-version-observability-signs-point-to-yes>
- Charity Majors, "Logs vs. Structured Events":
  <https://charity.wtf/2019/02/05/logs-vs-structured-events/>
- Jessica Kerr, "Span or Attribute in OpenTelemetry Custom Instrumentation":
  <https://www.honeycomb.io/blog/span-or-attribute-opentelemetry-custom-instrumentation>
- Mike Terhar, "Exotic Trace Shapes": <https://www.honeycomb.io/blog/exotic-trace-shapes>
- Jessica Kerr, "Ask Miss O11y: Tracing Is for Async, Too":
  <https://www.honeycomb.io/blog/tracing-is-for-async-too>
- Honeycomb, missing spans:
  <https://docs.honeycomb.io/troubleshoot/common-issues/data-in-honeycomb>
- Honeycomb, Refinery trace patterns:
  <https://docs.honeycomb.io/manage-data-volume/sample/honeycomb-refinery/diagnose>
- Tekton TEP-0124:
  <https://github.com/tektoncd/community/blob/main/teps/0124-distributed-tracing-for-tasks-and-pipelines.md>
- tektoncd/pipeline#9699: <https://github.com/tektoncd/pipeline/pull/9699>
- tektoncd/pipeline#10866: <https://github.com/tektoncd/pipeline/issues/10866>
- Argo Workflows tracing: <https://github.com/argoproj/argo-workflows/blob/main/docs/tracing.md>
- argoproj/argo-workflows#16899: <https://github.com/argoproj/argo-workflows/issues/16899>
- open-telemetry/semantic-conventions#1648:
  <https://github.com/open-telemetry/semantic-conventions/issues/1648>
- KEP-647: <https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/647-apiserver-tracing/README.md>
- KEP-5915: <https://github.com/kubernetes/enhancements/issues/5915>,
  design PR <https://github.com/kubernetes/enhancements/pull/5916>
- kubernetes-sigs/cluster-api#3760: <https://github.com/kubernetes-sigs/cluster-api/issues/3760>
- OTel Collector githubreceiver:
  <https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/githubreceiver/README.md>
- OTel CI/CD semantic conventions: <https://opentelemetry.io/docs/specs/semconv/cicd/cicd-spans/>
