# Hardware Templating, Reference Policy, and the Hardware Controller

Related: [REFERENCES.md](../REFERENCES.md), [v1alpha2 templating](../v1alpha2/templating.md),
[CAPTAINOS.md](../CAPTAINOS.md)

## 1. Problem

Every Hardware field is a literal value. The motivating case is per-machine OSIE files
(`spec.metadata.osieFiles`), which let Smee deliver arbitrary files to CaptainOS before its
networking starts, through either the ISO bootstrap slot or an extra iPXE initrd. The same
limits apply to every other string field (`userData`, hostnames, kernel parameters, and
so on), which leaves four gaps:

1. **Data that already lives elsewhere must be copied into the Hardware.** Network,
   bonding, VLAN, BGP and credential data commonly lives in other Kubernetes objects
   (ConfigMaps, Secrets, custom resources). Hardware References
   ([REFERENCES.md](../REFERENCES.md)) exist for exactly this, but only Workflow Templates
   can use them.
2. **Credentials are stored in plaintext in the Hardware spec.** The files routinely
   carry certificates and tokens, and anyone who can read a Hardware can read them.
3. **Binary content is impossible.** `contents` is a JSON string; bytes that are not
   valid UTF-8 are replaced with U+FFFD by the API machinery, so a DER certificate or
   firmware blob is silently corrupted.
4. **Reference policy is enforced by convention, not by construction.** The allow/deny
   rules live in `tink/controller/internal/workflow`, and the backend exposes
   `DynamicRead` and `DynamicClient` publicly. Any component holding the backend can read
   arbitrary objects without consulting the policy. Adding a second consumer of references
   (Smee) under this model would duplicate the policy and widen the bypass.

Two related problems are addressed because the design depends on them:

5. **The binary runs up to three informer caches over the same objects.** The kube
   backend, the Tink Controller's manager and Rufio's manager each run their own, and
   Hardware is cached in all three. Adding a cache of referenced objects would make it
   four.
6. **Hardware has no status conditions and no controller of its own.** A reference that
   fails to resolve is only visible in Smee's logs, at boot. In v1alpha2, Rufio's Machine
   object is removed and its responsibilities move to a Hardware controller that does not
   exist yet.

## 2. Goals

- Any string field in a Hardware `spec` can reference other Kubernetes objects through
  `spec.references`, and other fields of the same Hardware, using the same syntax and the
  same access policy as Workflow Templates. OSIE files are the first use case.
- Existing Hardware keeps working unchanged until an operator opts in.
- DHCP, iPXE script, ISO and metadata requests gain no template execution and no network
  I/O, at any fleet size.
- Reference policy is applied in exactly one place, which no consumer can bypass through
  the backend's API.
- Rendering is transparent to consumers: Smee and Tootles receive rendered values without
  knowing that rendering exists.
- Binary data can be delivered.
- Render failures are visible through the Kubernetes API before a machine boots.
- One informer cache per process.
- The components introduced here are the building blocks for the v1alpha2 implementation,
  not throwaway code.

## 3. Non-goals

- Templating `metadata`, `status`, `spec.references`, or the fields Hardware is looked up
  by (§5.2).
- Templating non-string fields. The CRD schema types them, so a template cannot be stored
  there.
- Changing the engine v1alpha1 Workflow Templates are rendered with. It stays the existing
  whole-text renderer; only its data sources change (§6.4).
- A validating admission webhook.
- Guaranteeing that a machine sees a consistent set of values when a Hardware or a
  referenced object is edited during a boot (§5.9).
- Moving Rufio's power and inventory reconciliation in v1alpha1 (§9.3).

## 4. Background

### 4.1 Where Hardware is read today

| Component | How it reads Hardware |
| --- | --- |
| Smee, Tootles, Tink Server, SecondStar | Shared `kube.Backend` (`FilterHardware`, `ReadHardware`), informer cache scoped to `--backend-kube-namespace` |
| Tink Controller | Its own controller-runtime manager built from `b.ClientConfig`, scoped to `--backend-kube-namespace` |
| Rufio | Its own controller-runtime manager built from `b.ClientConfig`, scoped to `--backend-kube-namespace` |
| UI | Per-request client using the user's credentials (or configured auto-login credentials); namespace visibility follows those credentials, not `--backend-kube-namespace` |

References are resolved only by the Tink Controller, per Workflow, in
`tink/controller/internal/workflow/reconciler.go`: each `spec.references` entry is
checked against the Quamina deny/allow lists, then fetched with the backend's
`DynamicRead` (a live, uncached GET).

### 4.2 Hardware writes

- Tink Server patches Hardware with `client.MergeFrom` against a deep copy.
- The Tink Controller toggles `allowPXE` with a full `Update` of the whole object
  (`setAllowPXE` in `tink/controller/internal/workflow/hardware.go`).
- Rufio server-side-applies `status.attributes.outOfBand` as field manager
  `machine-controller`.

### 4.3 `pkg/template/render` (from loom)

[`github.com/jacobweinstock/loom`](https://github.com/jacobweinstock/loom) renders Go
`text/template` expressions found in the string values of a YAML document. It is copied
into this repository as `pkg/template/render` (both projects are Apache-2.0) and maintained here from
then on; Tinkerbell does not depend on the external module. The copy leaves out what this
design never uses: YAML input and output (replaced by `render.Value`, §5.4) and type
inference (always off here, §5.3). That also removes loom's only dependency,
`goccy/go-yaml`.

What the render package does:

- Only string leaves containing a delimiter are selected for rendering, and their results
  remain strings. These replacements do not reshape the document, but template helpers
  can mutate exposed maps and slices, including adding or removing fields. Hardware's
  hardcoded protected paths are checked separately after rendering (§5.2).
- The document is exposed to its own templates under a configurable self key, and caller
  data is merged into the same root. Caller data is never re-templated.
- Fields may reference other templated fields; they are evaluated once each, in
  dependency order, and cycles are reported as `ErrReferenceCycle`.
- Functions are injected by the caller (`WithFuncs`), with `missingkey=error` by default,
  a per-field output cap, a best-effort per-field output-write deadline, and a cap on the
  combined output of one render. These are operational safeguards, not a security
  boundary (§5.3, §10).
- Errors carry the field path, e.g. `spec.metadata.osieFiles[0].contents`.

Behaviour verified against loom `main` while writing this design:

| Probe | Result |
| --- | --- |
| `{{ .self.hostname }}`, references data, multi-line values | Rendered correctly |
| Literal `{{` via `{{ "{{" }}` | Works (loom's documentation says there is no escape; `pkg/template/render` corrects it) |
| `"{{ \"0644\" }}"` with type inference on | Becomes the integer `420` |
| Raw bytes from `b64dec` | Produce YAML that cannot be parsed back (`control characters are not allowed`) |

The last two findings drive §5.3 and §5.4.

## 5. Design: rendering

### 5.1 User-facing API

Templating is enabled for the whole process by a flag, off by default:

| Flag | Helm value | Default |
| --- | --- | --- |
| `--backend-kube-hardware-templating-enabled` | `deployment.envs.globals.backendKubeHardwareTemplatingEnabled` | `false` |

It is off by default because existing Hardware already contains `{{` that is not a Go
template. `spec.userData` and `spec.vendorData` commonly carry cloud-init Jinja
(`{{ ds.meta_data.hostname }}`) or templates inside files they deliver, and CAPT copies
Cluster API bootstrap data unchanged into `spec.userData`. Go templates cannot parse
Jinja (`function "ds" not defined`), so enabling templating without first excluding or
escaping those values would stop them rendering (§5.8). Operators can exclude entire
strings with the skip annotation below, or write a literal `{{` as `{{ "{{" }}`.
v1alpha2 has no flag: templating is always on.

With the flag enabled, every string field under `spec` is a template, apart from the
exclusions in §5.2:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: machine1
  namespace: tinkerbell
spec:
  references:
    net:
      group: example.org
      version: v1
      resource: networkconfigs
      namespace: tinkerbell
      name: machine1
    creds:
      version: v1
      resource: secrets
      namespace: tinkerbell
      name: machine1-creds
  userData: |
    #cloud-config
    fqdn: {{ .hardware.spec.metadata.instance.hostname }}
  metadata:
    instance:
      hostname: '{{ .hardware.metadata.name }}.{{ .references.net.spec.domain }}'
    osieFiles:
      - path: network.json
        contents: '{{ .references.net.spec | toJson }}'
      - path: certificates/client.der
        mode: "0400"
        contents: '{{ index .references.creds.data "client.der" | b64dec }}'
```

Rules:

- There is no per-field opt-in. This matches Workflow Templates, and a forgotten opt-in
  would deliver `{{ ... }}` literally with no error. A literal `{{` is written as
  `{{ "{{" }}`. The skip annotation provides an explicit per-field opt-out.
- Available data:
  - `.hardware` — the Hardware being rendered. A field that references another templated
    field sees that field's rendered value (in the example, `userData` sees the rendered
    `hostname`). Reference cycles are an error. `.hardware.metadata` and
    `.hardware.status`, including `status.attributes`, are readable inputs only; their
    values are not interpreted as templates.
  - `.references.<name>` — the Hardware's own references, subject to policy (§6).
- Functions: Sprig's hermetic function map plus the existing Tinkerbell helpers
  (`toYaml`, `fromYaml`, `formatPartition`, `netmaskToPrefixLength`). The same map is used
  for Workflow Templates; it moves to a shared package.
- The API server validates the stored text, not the rendered value. A field with a CRD
  pattern or enum (for example `osieFiles[].mode`, pattern `^(0[oO])?0*[0-7]{1,3}$`)
  therefore cannot hold a template.
- With the flag disabled, nothing is rendered and every consumer receives the Hardware as
  stored, including `osieFiles` contents.

### 5.2 What is rendered

Only eligible string values under `spec` are rendered. Metadata and status are included
in the document as readable inputs, but are never rendered:

| Path | Readable as | Rendered | Why not |
| --- | --- | --- | --- |
| `spec.*` string values | `.hardware.spec...` | Yes | |
| `metadata` | `.hardware.metadata...` | No | `kubectl apply` stores a copy of the spec, templates included, in the `last-applied-configuration` annotation |
| `status` | `.hardware.status...` | No | Controller-observed state and hardware attributes are inputs, not template targets |
| `spec.references` | `.hardware.spec.references` | No | References must be known before rendering can start |
| `spec.interfaces[].dhcp.mac` | yes | No | Lookup key (MAC index) |
| `spec.interfaces[].dhcp.ip.address` | yes | No | Lookup key (IP index) |
| `spec.agentID` | yes | No | Lookup key (agent ID index) |
| `spec.metadata.instance.id` | yes | No | Lookup key (instance ID index) |

The built-in exclusions are declared in one `skippedHardwarePaths` list of full paths,
independent of `protectedHardwarePaths`. A path excludes its value and subtree; `[]` selects
array elements, as in `spec.interfaces[].dhcp.mac`. There is no regex or wildcard matching.
Adding or removing a built-in exclusion changes one entry in this list. Only strings under
`spec` are eligible for rendering; other root fields remain excluded regardless of the list.
Annotation exclusions below remain exact-match rather than using these subtree/array rules.

The lookup keys are indexed from the stored object (`pkg/backend/kube/index.go`), which is
how DHCP, Tootles and Tink Server find a Hardware. A template there would put the template
text into the index, so lookups would never match. Indexing rendered values instead would
make a machine's identity depend on other objects: editing a ConfigMap could change which
Hardware answers a DHCP request.

The renderer skips these fields, so a `{{` in one of them stays literal, exactly as it does
today. No CRD validation rule is added: rejecting `{{` there would tighten validation for
existing users, and on clusters without validation ratcheting it could reject updates to
objects that already exist.

#### Additional exclusions with an annotation

`metadata.annotations["tinkerbell.org/render-skip"]` is a JSON array of exact string-leaf
paths to exclude in addition to the built-in exclusions:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: machine1
  annotations:
    tinkerbell.org/render-skip: '["spec.userData", "spec.vendorData"]'
spec:
  userData: |
    ## template: jinja
    #cloud-config
    hostname: "{{ ds.meta_data.hostname }}"
```

Paths use the render engine's notation, such as `spec.userData` or
`spec.interfaces[0].dhcp.hostname`. Map keys containing punctuation are quoted, such as
`spec.someMap["a.b"]`. Matching is exact: no wildcards or subtree exclusions. A path that
does not name a present string leaf has no effect, so absent optional targets are allowed.
Duplicate paths are harmless. A missing annotation or `[]` adds no exclusions.

Malformed JSON, non-string entries, `null`, empty paths, and paths outside `spec` are render
errors, even when there are no templates. The annotation can add exclusions, not remove
the built-in ones. With templating disabled, the annotation has no effect.

Excluded strings are not parsed, dependency-analysed, or executed. Other fields can still
read them, for example `{{ .hardware.spec.userData }}`; the resulting string is not rendered
again. If all template delimiters occur in excluded fields, the original Hardware is
returned unchanged. This annotation affects Hardware rendering, not Workflow Template
rendering.

Skipping template interpretation is not an immutability guarantee. Sprig helpers such as
`set` can mutate maps exposed to the current rendering. Annotation paths add skipping only,
not protection. A helper may change an annotation-skipped field unless it also belongs to
the hardcoded protected set below.

#### Hardcoded protection after rendering

Protection is separate from leaf skipping. After template execution, before conversion
back to typed Hardware, the backend compares these hardcoded values against a private
pre-render snapshot:

- `apiVersion`, `kind`, and the complete `metadata` and `status` subtrees.
- The complete `spec.references` subtree.
- `spec.agentID` and `spec.metadata.instance.id`.
- Each interface's `dhcp.mac` and `dhcp.ip.address`, matched by array position.

The policy is declared in one `protectedHardwarePaths` list of full paths, including
`spec.interfaces[].dhcp.mac` and `spec.interfaces[].dhcp.ip.address`. Literal field names
are separated by dots; `[]` means iterate each array element in the original or final
document. There is no regex or wildcard matching. Adding or removing protection changes
one entry in this list, independently of the skip policy. This notation is internal;
annotation skip paths still require exact indexes such as `spec.interfaces[0].dhcp.mac`.

A change in value or presence rejects the render with an error naming the affected path,
without exposing values. Checks include originally absent fields and lookup keys in newly
added interfaces. Removing or replacing a parent cannot bypass these checks. Changes to
unprotected siblings remain allowed. Array positions are compared, not inferred interface
identities.

This is a final-state contract, not execution-time immutability: a helper may temporarily
change a protected value and restore it before rendering ends. Other fields can observe a
temporary value during that render. Helpers remain available; no restoration of protected
fields is performed, including status. A failed render returns no Hardware result.

Reference maps are deep-copied before template execution, so local mutations cannot change
the caller's references even on failure. The original Hardware also remains untouched.
Independent cache ownership at the Workflow handoff is still required (§5.7); rejecting
a result would not undo a mutation to a shared cache input.

### 5.3 Rendering with `pkg/template/render`

The backend renders with the render package using these options:

| Option | Value | Why |
| --- | --- | --- |
| `WithFuncs` | shared hermetic function map | Same functions as Workflow Templates |
| `WithMissingKeyError` | `true` | A denied or missing reference fails instead of rendering `<no value>` |
| `WithSelfKey` | `"hardware"` | `.hardware` is the object being rendered |
| `WithSkip` | the built-in and annotation paths in §5.2 | Excludes whole strings before parsing |
| `WithMaxOutputBytes` | package default (1 MiB) | Caps one field's output; the OSIE archive total is checked separately (§5.10) |
| `WithMaxTotalBytes` | package default (8 MiB) | Caps the combined output of one render, so many templated fields cannot each claim the per-field cap |
| `WithOutputDeadline` | package default (2s) | Best-effort check on output writes; it cannot interrupt a blocking function or non-writing template execution |

These limits are operational safeguards against mistakes such as an oversized value or a
runaway loop. They cap rendered output, not memory or CPU: a template can allocate or loop
without writing, for example by growing a variable in a `range`, and nothing interrupts
execution between writes. They are not a security boundary (§10).

The document passed to the render package is the object's `apiVersion`, `kind`, `metadata`,
`spec` and `status`, so templates can read `.hardware.metadata.name`,
`.hardware.spec...` and `.hardware.status.attributes...`. Metadata and status are skipped
as render targets, so literal template text in them is not interpreted.
Resolved references are passed as data under `references`. The same shape and options are
used for v1alpha1 and v1alpha2; only the list of lookup-key paths differs.

A rendered value is always a string. `pkg/template/render` has no type inference: the result is
decoded into a typed CRD, where a rendered `"0644"` must stay the string `"0644"`, not
become the integer `420`.

`pkg/template/render` adds an option that keeps a string leaf out of rendering while leaving it
readable through the self key:

```go
// WithSkip leaves unrendered every string value for which skip(path) returns true.
// Skipped values stay readable through the self key, unrendered.
func WithSkip(skip func(path string) bool) Option
```

Paths use loom's existing format, for example `spec.interfaces[0].dhcp.mac`.
`HasTemplates(doc, opts...)` uses the same leaf selection without parsing or modifying the
document. The backend uses the same skip predicate for detection and rendering.

### 5.4 Rendering a decoded tree (`pkg/template/render` addition)

`loom.Render` round-trips through YAML. For typed Kubernetes objects that is both lossy
(binary strings) and wasteful (marshal to YAML, parse, re-marshal, convert back).

loom already renders a decoded tree internally (`renderDoc(doc any, ...)`).
`pkg/template/render` adds a public function:

```go
// Value renders the templated string leaves of an already-decoded document
// (map[string]any / []any / scalars) in place and returns it.
func Value(doc any, data map[string]any, opts ...Option) (any, error)
```

The backend then renders without any serialization:

```text
typed Hardware
  -> runtime.DefaultUnstructuredConverter.ToUnstructured
  -> render.Value
  -> runtime.DefaultUnstructuredConverter.FromUnstructured
  -> rendered Hardware (in memory only)
```

The unstructured converter works by reflection, so Go strings, including non-UTF-8
bytes produced by `b64dec`, pass through unchanged. This is what makes binary delivery
work (§5.5). The map and slice types it produces (`map[string]interface{}`,
`[]interface{}`) are the types the render package already walks.

### 5.5 Binary contents

Rendered values only ever exist in memory in the backend and its consumers; they are never
written to an API object. With `render.Value`, a field whose template ends in `b64dec` is
delivered byte-for-byte, for example into the CPIO archive. The Hardware object itself only
stores the template text, which is always valid UTF-8.

### 5.6 Rendered and stored objects must not be confused

A rendered Hardware written back to the API server would replace the stored templates
with their output. Two measures prevent this:

- **Rendered Hardware only reaches read-only interfaces.** Smee's and Tootles' backend
  interfaces already have a single method, `FilterHardware`. With templating enabled, they
  are given a wrapper that implements only that method and returns the rendered
  `*tinkerbell.Hardware`, so no write method is reachable from them. The Tink Controller
  receives rendered Hardware as template data only (§6.4), never as an object it writes.
  A separate read-only domain type, which would make this a compile-time guarantee, is
  deferred to v1alpha2, where the consumers' types change anyway.
- **Write paths keep the stored type.** Tink Server's discovery and attribute paths, and
  every controller that writes Hardware, keep using the unrendered read methods.

### 5.7 Rendering off the request path

DHCP, iPXE script, ISO range and Tootles metadata requests all find their Hardware with
`FilterHardware`: an indexed lookup in the informer cache plus a copy, with no API calls.
ISO mounts alone issue thousands of these per machine. The requirement is that templating
adds no template execution and no network I/O to any of them.

Rendering therefore happens in the background, and requests only look results up.

**The render store.** The backend keeps a store of rendered Hardware, addressed by
namespace/name with a Hardware UID recorded on each entry. Lookup and last-successful
fallback both require the same UID: deletion and recreation at the same name must never
inherit the deleted object's result, even when queue events coalesce.
Each entry records the Hardware UID and `resourceVersion`, the rendered result, and the
last rendering error. Reference changes invalidate entries through the reverse index and
metadata informers rather than per-reference version snapshots in the entry.

**Background workers.** Hardware events come from the backend cache; reference events come
from a separate metadata-only informer factory. They add Hardware keys to a rate-limited
work queue, and a fixed pool of workers renders them:

- Hardware added, updated or deleted.
- A referenced object added, updated or deleted. The store keeps a reverse index from each
  referenced `(group, resource, namespace, name)` to the Hardware that reference it. The
  first time a new referenced resource appears, the store registers an event handler on
  the independent metadata factory's informer. This factory watches all namespaces,
  regardless of `--backend-kube-namespace`, so allowed cross-namespace references stay
  current. Its informers stop with the render context and are joined at store shutdown.
- A referenced Secret's metadata changing (§7.5).

Watch registration is serialized separately from entry access. A resource is marked
watched only after handler registration succeeds. Discovery or handler-registration
failure schedules a rate-limited Hardware retry even if the rendered result is valid;
that valid result remains available. Established informers use client-go's list/watch
reconnection rather than a second retry mechanism. Concurrent workers do not install
duplicate handlers.

Registration is not a snapshot-synchronization barrier: informers start asynchronously.
A live read can observe an object that is deleted before the initial LIST, leaving no
object for the informer to emit a deletion event for. The same gap can occur after watch
expiration when an object is created, read live, and deleted before a recovery relist;
`HasSynced()` remains true after the first synchronization.

The reference metadata client therefore requeues all current referrers of a resource after
every successful LIST, including empty initial lists and recovery relists. Reconciliation
is queued, not executed in the LIST callback, so workers never block on informer sync.
Hardware joining a pending watcher is included through the current reverse index. Failed
LISTs do not trigger this completion hook; client-go retries them. The factory uses explicit
LIST/WATCH rather than watch-list initialization so no snapshot path bypasses the hook.

Hardware update handling must include changes to the skip annotation, other metadata
read by templates, and status attributes. For CRDs, metadata-only updates do not increment
`metadata.generation`; Hardware status updates do not increment it either. A generation-only
predicate would miss these inputs. Updates compare private normalized documents, ignoring
only `metadata.resourceVersion`, `metadata.managedFields`, and the renderer-owned `Rendered`
condition. Other status changes and mixed condition-plus-input changes still enqueue
rendering. The `Rendered` condition is bookkeeping, not a render invalidation input.
Reference metadata updates do not use this filter: their resource versions signal data
changes even when the metadata payload otherwise looks identical.

**The request path.** `FilterHardware` does the same indexed lookup as today, then one map
lookup in the store by namespace/name, with UID validation before any cached result is
used. No template runs and no network call happens. Informer observations and workers
prepare the render-needed decision, keyed by Hardware UID and resource version. During
startup/edit windows, a matching decision avoids request-path conversion and scanning.
A snapshot not yet observed, or one with a different UID/version, uses the existing
skip-aware scan to preserve immediate literal passthrough and annotation validation.

**Fast path.** When templating is disabled, or a Hardware has no `{{` in any non-skipped
string leaf, the store records that the rendered result is the stored object and keeps no
copy. Detect this before resolving references for Hardware rendering; Workflow Template
references are resolved independently. Fleets that do not use templating pay nothing
beyond the map lookup.

A lookup benchmark cycles through 10,000 Hardware in literal, skipped-Jinja, and templated
fleets, with steady-state, unobserved startup/edit, and observed startup/edit cases.
On the development host, unobserved scans took roughly 32-60 microseconds and over 100
allocations per call; observed literal/Jinja catch-up cases took approximately
0.1-0.4 microseconds with zero allocations. Templated results still incur the required
copy. These are local measurements, not a cross-machine latency guarantee.

**Referenced data.** PR6 workers read referenced objects live through the reference
resolver; the independent informers retain metadata only, never full Secret contents.
Later shared-data-cache work must preserve the watchers' cross-namespace coverage and
live Secret reads (§7.5). Reference data held by the store is limited to rendered results,
not all objects of a referenced type.

**Shared references.** When an object referenced by many Hardware changes, all of them are
queued. The queue is rate limited, and requests keep being answered from the previous
renderings until each new one is ready.

Cache-owned Hardware and reference maps must not be exposed directly to template execution.
Each independent Hardware or Workflow rendering receives private mutable inputs, so a
helper mutation cannot affect another render through a shared reference or cached result.
Workflow task workers become `status.tasks[].agentID` and `status.agentID`, which drive
agent targeting; sharing mutable render inputs could therefore contaminate later routing.
This requires ownership isolation, not a prohibition on helper mutations within one render.

**Every replica renders.** Smee and Tootles run in every replica, not only on the leader,
so each replica keeps its own store. Rendering is deterministic, so replicas agree.

**Not ready.** A newly created Hardware that needs rendering has no entry until its first
render completes, typically within milliseconds. Until then, consumers treat it as not
found; DHCP clients retry within seconds.

**Atomic replacement.** Each entry is replaced whole, so a request never sees some fields
from one rendering and some from another.

**Notifications.** `Changes()` exposes a one-slot wake channel; `TakeChanges()` atomically
drains the coalesced set of changed Hardware keys. Result replacement, render failure,
recovery, and deletion mark keys pending without blocking workers. Slow consumers keep
the latest key set rather than a per-event backlog; consumers reconcile against the latest
store state, including UID validation. The wake channel closes after workers stop, and
also on startup failure. PR8 integrates this with the Hardware status controller.

**Metrics.** PR7 supplies the registerer for one store using the existing metrics registry.
The store registers `tinkerbell_hardware_render_queue_depth`,
`tinkerbell_hardware_render_duration_seconds`, `tinkerbell_hardware_render_attempts_total`
(only the bounded `result=success|error` label), and
`tinkerbell_hardware_render_fallback_entries`. Attempts include watch-registration failure
and read errors; the fallback gauge counts entries serving last-good data after a rendering
error and decreases on recovery or deletion. No Hardware or reference names are labels.

Informers on referenced types need `list` and `watch` RBAC for those types, not only
`get`. The independent factory lists and watches across all namespaces, so those permissions
must be granted cluster-wide for referenced types. Operators grant them through the chart's
existing `rbac.additionalRoleRules`; the
templating documentation states that referenced types need all three verbs:

```yaml
rbac:
  additionalRoleRules:
    - apiGroups: ["example.org"]
      resources: ["networkconfigs"]
      verbs: ["get", "list", "watch"]
```

### 5.8 Render failures

If any field fails to render, the whole render fails. Falling back field by field would
serve raw template text to a machine.

- **A Hardware that has rendered successfully before** keeps serving its last successful
  rendering. The store records the error, and the Hardware controller sets `Rendered` to
  `False` with the reason, the failing field, and which rendering is being served (§8.3).
  A typo in a reference therefore does not take DHCP renewals away from running machines.
  The cost is that stale values, including old Secret data, keep being served until the
  error is fixed; the condition and the metric make that visible.
- **A Hardware that has never rendered successfully** is not ready (§5.7): consumers treat
  it as not found, and Smee logs the render error.
- **The last successful rendering is held in memory only.** After a restart, a Hardware
  whose templates are currently failing has no previous rendering to fall back on, and is
  not ready until it is fixed.

An OSIE archive that is too large (§5.10) is not a render failure. It is reported the same
way, but the current rendering keeps being served.

### 5.9 Edits during a boot

A BMC assembles one virtual-media mount from many range requests. If a Hardware or a
referenced object changes partway through, the machine can receive a mix of old and new
bytes. This is not guarded against; the templating documentation states: "Editing a
Hardware or its referenced objects while a machine is booting from it may deliver a mix of
old and new values. Avoid editing during provisioning."

### 5.10 Size limits

- Per field: the render package's output cap (§5.3).
- Per render: the render package's cap on the combined output of all fields (§5.3).
- OSIE archive: after rendering, the render store builds the
  bootstrap archive from `osieFiles` and checks its size against
  `--backend-kube-bootstrap-slot-capacity` (Helm
  `deployment.envs.globals.backendKubeBootstrapSlotCapacity`), default 4 MiB, the size
  CaptainOS reserves and expected to stay stable. The check runs whether or not templating
  is enabled, because literal contents can be too large too. An oversized archive is
  reported through the `Rendered` condition (§8.3).

The check does not change what is served. The slot limit applies only to ISO delivery; the
iPXE initrd has no limit, so treating an oversized archive as a render failure would cut
off DHCP and iPXE boots for a problem only ISO delivery has. Smee keeps checking the
archive against the capacity in the source ISO's published layout before serving any bytes
(`smee/internal/iso/iso.go`), and that remains what rejects an oversized ISO. If an ISO is
built with a different slot size, the flag must be set to match, or the condition and
Smee's check disagree.

## 6. Design: reference policy in the kube backend

### 6.1 Placement

Policy evaluation and reference resolution move from `tink/controller/internal/workflow`
into `pkg/backend/kube`. The backend is the only component that resolves references, and
the only path to resolution applies the policy.

### 6.2 Closing the bypasses

- `DynamicRead` and the `DynamicClient` field become unexported. The only exported way
  to read a referenced object is the policy-checked `ResolveReferences`.
- A `depguard` rule forbids importing `k8s.io/client-go/dynamic` outside
  `pkg/backend/kube`.
- The Tink Controller stops calling `DynamicRead` directly (§6.4).

This makes the backend the enforcement point for code in this repository. It is not a
security boundary against a component that builds its own client from the shared
`ClientConfig`; RBAC on the ServiceAccount remains that boundary.

### 6.3 API

```go
// ResolveReferences returns the Hardware's references that the policy allows,
// keyed by reference name. Denied or missing references are reported in the
// returned error and omitted from the map.
func (b *Backend) ResolveReferences(ctx context.Context, hw *tinkerbell.Hardware) (map[string]any, error)
```

The render store (§5.7) and the Tink Controller use this policy-checked resolver.

### 6.4 Tink Controller

The Workflow reconciler stops evaluating policy and calling `DynamicRead` itself. It builds
its template data from the backend. When enabled, `RenderedHardware` reads the store's
latest successful Hardware result; the Workflow reconciler does not render Hardware itself.
An unready Hardware returns a not-found/readiness error before Workflow references are read.
Workflow references are independently resolved through `ResolveReferences(ctx, storedHW)`
under Hardware-wide policy; they are not removed or replaced by the store's cached result.

The existing whole-text Workflow renderer remains unchanged. Its private input contains
the Hardware data under `.hardware` and legacy `.Hardware`, plus resolved objects under
`.references`. Reference declarations under `.hardware.spec.references` remain readable.
With templating disabled, `RenderedHardware` returns the stored object, so existing
Workflow references, output and condition-error messages retain their behavior. Neither
the rendered Hardware nor template helper mutations are written back to its CR.

### 6.5 Hardware-wide rules

The Quamina event contains the source Hardware and the referenced object:

```json
{
  "source": {"name": "machine1", "namespace": "tinkerbell"},
  "reference": {"name": "machine1-creds", "namespace": "tinkerbell", "group": "", "version": "v1", "resource": "secrets"}
}
```

Reference policy applies to the Hardware regardless of which component reads it. Smee
serves rendered values to any machine presenting the right MAC address, and Tootles to
any client with the right IP address, both unauthenticated. A rule allowing a Secret for
a Hardware also permits it to appear in values served over the provisioning network.
The render store keeps one entry per Hardware.

### 6.6 Configuration

The backend reads Hardware reference rules from global flags. The former flag and
environment names remain deprecated aliases, while the Helm value paths stay unchanged.
v1alpha2 Task and Workflow references use separate policy rule sets:

- `--backend-kube-hardware-reference-allow-list-rules` /
  `deployment.envs.tinkController.referenceAllowListRules`
- `--backend-kube-hardware-reference-deny-list-rules` /
  `deployment.envs.tinkController.referenceDenyListRules`

The default remains deny-all: references fail to render until an allow rule is configured,
and the Hardware condition (§8.3) says so.

## 7. Design: one informer cache per process

### 7.1 Approach

The kube backend owns a single `cache.Cache` and is the only component that starts it.
The Tink Controller, Rufio and the Hardware controller each keep their own manager (and
therefore their own leader-election lease), but create it with `NewCache` returning a
wrapper around the shared cache:

```go
// sharedCache lets a manager use a cache that another component starts.
type sharedCache struct{ cache.Cache }

// Start blocks until ctx is done; the owner starts the underlying informers.
func (s sharedCache) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
```

Reads, `WaitForCacheSync` and `IndexField` pass through to the shared cache.

This relies on controller-runtime behaviour verified in v0.24.1:

- `manager.Options.NewCache` accepts a custom cache.
- Starting an informer cache twice returns `informer already started`, so exactly one
  owner may start it.
- A manager starts its caches before leader election, so non-leader replicas still have a
  working cache (Smee, Tootles and Tink Server need it regardless of leadership).

A single manager for the whole process was considered and rejected: it would force the
Tink Controller, Rufio and the Hardware controller to share one lease.

### 7.2 Startup order

Field indexes must be registered before the cache starts. `cmd/tinkerbell` changes from
"create backend and start it immediately" to:

1. Create the backend and its cache (not started).
2. Create each enabled controller's manager with the shared cache and call its
   `SetupWithManager`, which registers indexes through the shared cache.
3. Start the backend's cache, then the managers and servers.

The backend's indexes become the union of what enabled components need.

### 7.3 Scope and scheme

- The cache's scheme is the union of all components' schemes (core, `tinkerbell.org`,
  `bmc.tinkerbell.org`); the backend already registers all three.
- The backend, Tink Controller and Rufio all use `--backend-kube-namespace`: empty means
  cluster-wide, and a non-empty value scopes each to that namespace. Their cache scopes
  therefore match, allowing the cache to be shared. The UI uses request credentials and
  remains outside this cache and scope contract.
- Referenced objects are cached in the same cache as unstructured objects (§5.7).

### 7.4 Limits

The saving applies to the single binary. Components deployed as separate processes each
keep their own cache.

### 7.5 Secrets are never cached in full

Rufio reads BMC credential Secrets with `mgr.GetClient()` and default options
(`resolveAuthSecretRef` in `rufio/internal/controller/kube.go`, `retrieveHMACSecrets` in
`machine.go`). Those reads are served from an informer, so Rufio today holds every Secret
it can list in memory. With the chart's defaults (`rbac.type: ClusterRole`,
`rbac.secrets.enabled: true`) that is every Secret in the cluster, including Helm release
and ServiceAccount token Secrets, inside a pod limited to 128Mi. Reference rendering would
add another reader of Secrets.

Neither use needs full Secrets in memory. Rufio reads a handful of credentials per
`powerCheckInterval`; rendering needs a Secret's data only when it has changed. So in the
shared cache:

- **Secret data is always read live.** Every client built on the shared cache sets
  `client.Options.Cache.DisableFor: []client.Object{&corev1.Secret{}}`. controller-runtime
  applies `DisableFor` by GVK before its unstructured check, so typed and unstructured
  Secret reads both go to the API server. Rufio's behaviour is unchanged apart from where
  the bytes come from.
- **Change detection uses metadata only.** Where a Secret's changes must be observed (the
  render store, §5.7), the cache holds a metadata-only informer
  (`metav1.PartialObjectMetadata` for Secrets). It stores names, labels and
  `resourceVersion`s, not data. The render store keeps the data only of Secrets that some
  Hardware references, and re-fetches it live only when the `resourceVersion` moves.
  It must read that metadata from the cache itself (`cache.Get`), not through a client:
  a `PartialObjectMetadata` for Secrets carries the Secret GVK, so `DisableFor` would send
  a client read of it to the API server too.

The chart already grants `get`, `list` and `watch` on Secrets, which the metadata informer
needs.

## 8. Design: the Hardware controller

### 8.1 A new top-level component

A new component, `hardware/`, alongside `rufio/`, `smee/` and `tink/`:

- Enabled by `--enable-hardware-controller` (default `true`), with its own Helm values.
- Started only when templating is enabled. In v1alpha1 reporting render results is its only
  job, and starting it otherwise would give every existing install a new Lease and status
  writes on every Hardware.
- Its own manager, using the shared cache (§7), with lease ID
  `hardware-controller.tinkerbell.org`.
- Reconcilers in `hardware/internal/controller`, each an independent unit registered on
  the manager, each writing a disjoint part of the status with server-side apply under its
  own field manager.

It is a top-level component rather than a reconciler inside the Tink Controller because
v1alpha2 removes Rufio's Machine object and moves BMC power and inventory reconciliation
onto Hardware. That work belongs to a Hardware controller, not to the Workflow controller.

### 8.2 Reconcilers

| Reconciler | v1alpha1 | v1alpha2 |
| --- | --- | --- |
| Rendering status → `Rendered` condition | This design | Carried over |
| BMC power state | Stays in Rufio's Machine controller | Moves here |
| Out-of-band inventory | Stays in Rufio's Machine controller | Moves here |

Inventory stays in Rufio in v1alpha1 because it reuses the BMC connection that the Machine
reconciler opens for power checks; moving it alone would mean a second connection.

### 8.3 Rendering status reconciler

The reconciler does not render. It reports what the backend's render store (§5.7) produced,
so what it reports is exactly what Smee and Tootles serve, and nothing is rendered twice.

- The store notifies the reconciler after each render through a `source.Channel`, which
  enqueues the Hardware. Changes to referenced objects therefore reach the reconciler
  through the store's informer event handlers; the reconciler adds no watches of its own.
- The leader's store is the one reported. Every replica's store renders the same result.
- Sets the `Rendered` condition, which means the Hardware's rendered values are valid for
  the Hardware rendering:

| Status | Reason | When |
| --- | --- | --- |
| `True` | `Rendered` | The Hardware rendering succeeded and passed validation |
| `True` | `NoTemplates` | Nothing in the Hardware needs rendering, and it passed validation |
| `False` | `ReferenceDenied` | A reference is denied by policy |
| `False` | `ReferenceNotFound` | A referenced object does not exist |
| `False` | `TemplateError` | The render package returned a `*FieldError` |
| `False` | `ArchiveTooLarge` | The OSIE archive exceeds `--backend-kube-bootstrap-slot-capacity` (§5.10) |

When `False`, the message names the failing field path and says what is
being served: the previous rendering after a render failure (§5.8), or the current one for
`ArchiveTooLarge`, in which case ISO delivery fails and iPXE delivery is unaffected.

- Uses the normal (unrendered) client; it only writes status.

If the component is disabled, rendering still works (it is in the backend), but failures
are only visible in logs and metrics. The documentation states this.

### 8.4 Conditions API

Hardware status conditions use `metav1.Condition` in both API versions:

```go
// v1alpha1 and v1alpha2
type HardwareStatus struct {
	// ...existing fields...

	// Conditions are the latest observations of the Hardware's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

- v1alpha1 gains the field; it is net new.
- v1alpha2 replaces `[]bmc.Condition` with `[]metav1.Condition` on both `HardwareStatus`
  and `JobStatus`. There is no v1alpha2 implementation yet, so there is nothing to
  migrate. `bmc.Condition`, `ConditionStatusOn`/`ConditionStatusOff`, `HasConditionStatus`
  and `SetCondition` are removed; `meta.SetStatusCondition` and
  `meta.IsStatusConditionTrue` replace them.
- Power state is not a condition. v1alpha2 represents it as a dedicated status field when
  the power reconciler moves in (§8.2).

`+listType=map` with `listMapKey=type` lets each reconciler server-side-apply only its own
condition type without taking ownership of the others.

## 9. Related changes

### 9.1 `allowPXE` without a full update

`setAllowPXE` replaces `cc.Update(ctx, h)` with:

```go
original := h.DeepCopy()
// ...set allowPXE on each interface...
cc.Patch(ctx, h, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
```

The optimistic lock keeps today's conflict-and-retry behaviour. `interfaces` has no
`listType`, so the merge patch replaces that array, but no longer rewrites `osieFiles`,
metadata or any other field. This is independent of rendering and can land first.

### 9.2 Smee

Smee's handlers receive the rendered read-only type from §5.6 instead of
`*tinkerbell.Hardware`; `bootstrap.Files` keeps treating `Contents` as bytes. A Hardware that
is not ready (§5.7) is handled like one that is not found. Smee never serves unrendered
template text.

### 9.3 Rufio

In v1alpha1, Rufio uses the shared cache, registers its indexes before the cache starts,
and reads Secrets live instead of from a cluster-wide Secret informer (§7.5). Its
reconciliation logic is unchanged.

### 9.4 Tootles

Tootles serves instance metadata, `userData` and `vendorData` from the rendered read-only
type, with the same not-ready handling as Smee.

## 10. Security

- **Template injection.** Anyone who can edit a Hardware can execute templates in any of
  its `spec` string fields, against that Hardware's resolved references and nothing else.
  Data passed to the render package is never re-templated. Functions are the hermetic set only.
- **Confused deputy.** The backend reads referenced objects with its own ServiceAccount on
  behalf of whoever wrote the Hardware. The allow/deny policy, default deny, is the control
  that stops a Hardware author reading objects they could not read directly.
- **Network exposure.** Rendered values are served by Smee to any client presenting the
  Hardware's MAC address, and by Tootles to any client with its IP address. Treat anything
  a Hardware may resolve as disclosed to the provisioning network; write Hardware-wide
  rules accordingly (§6.5).
- **Stale data.** After a render failure, the previous rendering, including any Secret
  values it contains, keeps being served (§5.8). Rotating a Secret does not take effect
  for a Hardware whose render is failing.
- **Resource exhaustion.** Rendering is not sandboxed. The output caps and per-field
  deadline (§5.3) are operational safeguards against mistakes; they do not bound memory or
  CPU, so a Hardware author can exhaust the backend's process. Hardware authors are
  therefore trusted to the same degree as for the rest of the Hardware, which already
  controls what a machine boots. If that changes, rendering needs a real boundary, such as
  a subprocess with memory and time limits. Rendering happens in rate-limited background
  workers, so request volume never drives render volume.
- **RBAC.** Caching referenced types requires `get`/`list`/`watch` on them. Operators
  grant these per type through `rbac.additionalRoleRules`; nothing is granted by wildcard.
  Secret data is never held in the cache (§7.5).

## 11. v1alpha2 alignment

| Concern | v1alpha1 (this design) | v1alpha2 |
| --- | --- | --- |
| Enablement | `--backend-kube-hardware-templating-enabled`, off by default | Always on |
| Templated Hardware fields | All `spec` string fields except §5.2 | Same, with v1alpha2's lookup keys |
| Document and self key | `metadata` + `spec`, `.hardware` | Same |
| Renderer | `render.Value` with `WithSkip` | Same |
| Where rendering runs | Background render store in the kube backend | Same |
| Render failures | Last successful rendering served | Same |
| Policy and resolution | kube backend | kube backend |
| Read type for consumers | rendered `*tinkerbell.Hardware` behind `FilterHardware`-only wrappers | read-only domain type |
| Hardware controller | `Rendered` condition | plus BMC power and inventory |
| Conditions | `metav1.Condition` | `metav1.Condition` (Hardware and Job) |

The v1alpha2 Workflow and Task rendering described in
[v1alpha2 templating](../v1alpha2/templating.md) uses the same render package configuration and
function map, rendered in the Tink Controller from the rendered Hardware and the
references the backend resolves.

## 12. Alternatives considered

| Alternative | Why not |
| --- | --- |
| `valueFrom.secretKeyRef` / `configMapKeyRef` on each file | A second reference mechanism with separate syntax and access control; limited to Secrets and ConfigMaps; cannot combine sources into one file. References plus `b64dec` cover the same cases. |
| Render in the Tink Controller into an owned Secret, served by Smee | Snapshot and binary safety are built in, but consumers must change, the policy stays outside the backend, and it adds an object per Hardware. |
| Render in Smee | Policy would be duplicated per consumer and Smee would need its own dynamic access. |
| Opt-in templating per file | Adds API surface; a forgotten opt-in delivers `{{ }}` literally with no error. |
| Templating only `osieFiles` | Leaves every other field without references; the performance concern it avoided is addressed by rendering in the background (§5.7). |
| Opt-in per Hardware (annotation) | Rejected in favour of one process-wide switch. |
| Templating always on in v1alpha1 | Breaks existing `userData`/`vendorData` containing Jinja, including CAPT's, on upgrade. |
| Rendering on request, with a cache | A cache miss would put template execution, and for Secrets network I/O, on the DHCP and iPXE path. |
| Indexing rendered lookup keys | A machine's identity would depend on other objects; a ConfigMap edit could change which Hardware answers DHCP. |
| Failing closed on render errors | One mistake in a template or reference would take a running machine off the network. |
| One manager for the whole process | Forces one leader-election lease across unrelated controllers. |
| Admission webhook with SubjectAccessReview | Not wanted; a Hardware controller condition gives the feedback without webhook operations. |
| Hardware reconciler inside the Tink Controller | Works for v1alpha1, but the v1alpha2 BMC reconcilers would then have to move out again. |

## 13. Implementation phases

Each phase is a separate pull request of at most 600 lines of non-test Go, and none changes
CLI flags, CRDs or Helm values incompatibly.

1. `setAllowPXE` merge patch (§9.1).
2. `pkg/template/render`: trimmed copy of loom with `Value` and `WithSkip` (§4.3, §5.3, §5.4).
3. Template functions moved to a shared package (§5.1).
4. Reference policy in the backend: `ResolveReferences`, private
   `DynamicRead`/`DynamicClient`, `depguard` rule (§6).
5. The renderer: stored Hardware and references to rendered Hardware (§5.2–§5.4).
6. The render store (§5.7, §5.8).
7. `--backend-kube-hardware-templating-enabled` and the wiring of Smee, Tootles and the Tink
   Controller to rendered data (§5.1, §5.6, §6.4, §9).
8. v1alpha1 `status.conditions` and the Hardware controller (§8).
9. The shared cache and live Secret reads (§7).
10. `metav1.Condition` for v1alpha2 Hardware and Job (§8.4).

The OSIE archive check, `--backend-kube-bootstrap-slot-capacity` and the `ArchiveTooLarge`
reason (§5.10) are delivered with the bootstrap CPIO and `osieFiles` work, on top of these.

Documentation: how to enable templating and exclude or escape existing `{{` before doing so, which
fields can be templated, the edit-during-boot note, last-successful fallback and its
behaviour across restarts, reference rules, and `rbac.additionalRoleRules` for referenced
types.

## 14. Decisions

Questions raised during review, and how they were settled:

| Question | Decision |
| --- | --- |
| Where does loom live? | Copied into `pkg/template/render` and maintained in this repository (§4.3). |
| Which fields can be templated? | Every `spec` string field, except `spec.references`, the lookup keys, and exact paths in the skip annotation (§5.2). |
| How can Jinja payloads stay literal? | List their exact paths in `tinkerbell.org/render-skip`, a JSON-array annotation (§5.2). |
| How is templating enabled in v1alpha1? | A process-wide flag, `--backend-kube-hardware-templating-enabled`, off by default (§5.1). |
| How is request latency protected? | Rendering runs in background workers; requests only look up results (§5.7). |
| What happens when a render fails? | The last successful rendering keeps being served, and the `Rendered` condition reports the failure (§5.8). |
| Should Rufio's Secret reads use the cache? | No. Secret data is read live; Secret changes are observed through a metadata-only informer (§7.5). |
| How are reference changes detected? | Informer event handlers registered by the render store at runtime per referenced GVK (§5.7). No periodic resync. |
| How do operators grant access to referenced types? | The chart's existing `rbac.additionalRoleRules`, with `get`, `list` and `watch` (§5.7). |
| How is an oversized OSIE archive reported? | The render store checks it against `--backend-kube-bootstrap-slot-capacity`, default 4 MiB, and reports it through `Rendered`. It does not change what is served (§5.10). |
| Does OSIE delivery get its own condition? | No. `Rendered` covers it; an `OSIEFilesReady` condition would duplicate it. |
| Which condition type? | `metav1.Condition` for v1alpha1 and v1alpha2 Hardware, and v1alpha2 Job (§8.4). |
| Are the reference-rule flags renamed? | Yes. The backend reads `--backend-kube-hardware-reference-*`; the old names are deprecated aliases, and Helm value paths stay unchanged (§6.6). |
| Is `{{` rejected in lookup-key fields? | No. The renderer skips them, so they behave as today (§5.2). |
| Does the Hardware controller run when templating is off? | No (§8.1). |
| Is the cache always shared? | Only when every component watches the same scope, the default (§7.3). |
| Do Smee and Tootles get a new domain type? | Not in v1alpha1. They get `FilterHardware`-only wrappers returning rendered Hardware (§5.6). |

## 15. Testing

- **Render package:** `Value` preserves non-UTF-8 bytes; the leaf walker handles the
  unstructured converter's types; a rendered `"0644"` stays a string; `WithSkip` leaves
  skipped values unrendered but readable; `HasTemplates` uses the same selection without
  parsing or modifying the document.
- **Backend:** policy is applied for every Hardware; denied and missing references are
  distinct errors; `metadata`, `status`, `spec.references` and lookup keys are not template
  targets; annotation skips preserve Jinja, compose with built-in exclusions, and reject
  malformed configuration; final changes to hardcoded protected fields reject rendering
  before typed conversion; annotation skips alone do not protect fields; original Hardware
  and references remain unchanged on success and failure; with the flag off, every consumer
  receives the stored object.
- **Render store:** a referenced object's change re-renders exactly the Hardware that
  reference it; a failing render keeps serving the previous one and reports it; a Hardware
  that never rendered is not found; entries are replaced atomically; an oversized OSIE
  archive is reported but the current rendering is still served, including for DHCP and
  iPXE. Annotation, template-readable metadata and attribute updates invalidate entries
  even when generation is unchanged; helper mutations cannot leak across independent
  renders through shared cached Hardware or references.
- **Request latency:** a benchmark of `FilterHardware` with 10,000 Hardware, templated and
  untemplated, stays within noise of today's baseline, with no API calls on the request
  path.
- **Compatibility:** with default flags and Helm values, `helm template` output changes only
  by new environment variables set to their defaults, and no Lease or Hardware status
  write is created.
- **Bypass guard:** the `depguard` rule fails CI when `k8s.io/client-go/dynamic` is
  imported outside `pkg/backend/kube`.
- **Shared cache:** envtest with the Tink Controller, Rufio and the Hardware controller on
  one cache; only one informer per GVK; indexes registered after cache start fail loudly;
  no full Secret informer exists, and Secret reads reach the API server.
- **Hardware controller:** each condition reason in §8.3, including recovery after a
  referenced object is created.
- **Smee:** an ISO range request and an iPXE initrd for a Hardware with a `b64dec` file
  carry identical, byte-exact contents.
- **Workflow:** a Workflow Template reading a templated Hardware field sees the rendered
  value.
- **Regression:** `setAllowPXE` leaves `osieFiles` untouched when another writer changes it
  concurrently.
