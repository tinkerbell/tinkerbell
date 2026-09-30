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
| Tink Controller | Its own controller-runtime manager built from `b.ClientConfig`, cluster-wide cache |
| Rufio | Its own controller-runtime manager built from `b.ClientConfig`, cluster-wide cache |
| UI | Its own client built from `b.ClientConfig` |

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

### 4.3 loom

[`github.com/jacobweinstock/loom`](https://github.com/jacobweinstock/loom) renders Go
`text/template` expressions found in the string values of a YAML document. It is copied
into this repository as `pkg/loom` (both projects are Apache-2.0) and maintained here from
then on; Tinkerbell does not depend on the external module. The copy leaves out what this
design never uses: YAML input and output (replaced by `RenderValue`, §5.4) and type
inference (always off here, §5.3). That also removes loom's only dependency,
`goccy/go-yaml`.

What loom does:

- Only string leaves containing a delimiter are rendered; the document's structure cannot
  be changed by a template.
- The document is exposed to its own templates under a configurable self key, and caller
  data is merged into the same root. Caller data is never re-templated.
- Fields may reference other templated fields; they are evaluated once each, in
  dependency order, and cycles are reported as `ErrReferenceCycle`.
- Functions are injected by the caller (`WithFuncs`), with `missingkey=error` by default,
  and a per-field output cap and time budget.
- Errors carry the field path, e.g. `spec.metadata.osieFiles[0].contents`.

Behaviour verified against loom `main` while writing this design:

| Probe | Result |
| --- | --- |
| `{{ .self.hostname }}`, references data, multi-line values | Rendered correctly |
| Literal `{{` via `{{ "{{" }}` | Works (loom's documentation says there is no escape; `pkg/loom` corrects it) |
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
Jinja (`function "ds" not defined`), so enabling templating without first escaping those
values would stop them rendering (§5.8). Operators enable it once their Hardware writes a
literal `{{` as `{{ "{{" }}`. v1alpha2 has no flag: templating is always on.

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
  `{{ "{{" }}`.
- Available data:
  - `.hardware` — the Hardware being rendered. A field that references another templated
    field sees that field's rendered value (in the example, `userData` sees the rendered
    `hostname`). Reference cycles are an error.
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

Only string values under `spec` are rendered. Everything else is either not part of the
document or readable but never rendered:

| Path | Readable as | Rendered | Why not |
| --- | --- | --- | --- |
| `spec.*` string values | `.hardware.spec...` | Yes | |
| `metadata` | `.hardware.metadata...` | No | `kubectl apply` stores a copy of the spec, templates included, in the `last-applied-configuration` annotation |
| `status` | — | No | Written by controllers, not authored |
| `spec.references` | `.hardware.spec.references` | No | References must be known before rendering can start |
| `spec.interfaces[].dhcp.mac` | yes | No | Lookup key (MAC index) |
| `spec.interfaces[].dhcp.ip.address` | yes | No | Lookup key (IP index) |
| `spec.agentID` | yes | No | Lookup key (agent ID index) |
| `spec.metadata.instance.id` | yes | No | Lookup key (instance ID index) |

The lookup keys are indexed from the stored object (`pkg/backend/kube/index.go`), which is
how DHCP, Tootles and Tink Server find a Hardware. A template there would put the template
text into the index, so lookups would never match. Indexing rendered values instead would
make a machine's identity depend on other objects: editing a ConfigMap could change which
Hardware answers a DHCP request.

The renderer skips these fields, so a `{{` in one of them stays literal, exactly as it does
today. No CRD validation rule is added: rejecting `{{` there would tighten validation for
existing users, and on clusters without validation ratcheting it could reject updates to
objects that already exist.

### 5.3 Rendering with loom

The backend renders with loom using these options:

| Option | Value | Why |
| --- | --- | --- |
| `WithFuncs` | shared hermetic function map | Same functions as Workflow Templates |
| `WithMissingKeyError` | `true` | A denied or missing reference fails instead of rendering `<no value>` |
| `WithSelfKey` | `"hardware"` | `.hardware` is the object being rendered |
| `WithSkip` | the paths in §5.2 | New in `pkg/loom`, see below |
| `WithMaxOutputBytes` | loom default (1 MiB) | Bounds one field; the OSIE archive total is checked separately (§5.10) |
| `WithRenderTimeout` | loom default (2s) | |

The document passed to loom is the object's `apiVersion`, `kind`, `metadata` and `spec`,
so that templates address fields as `.hardware.metadata.name` and `.hardware.spec...`, as
the [v1alpha2 templating](../v1alpha2/templating.md) doc specifies. `status` is omitted.
Resolved references are passed as data under `references`. The same shape and options are
used for v1alpha1 and v1alpha2; only the list of lookup-key paths differs.

A rendered value is always a string. `pkg/loom` has no type inference: the result is
decoded into a typed CRD, where a rendered `"0644"` must stay the string `"0644"`, not
become the integer `420`.

`pkg/loom` adds an option that keeps a string leaf out of rendering while leaving it
readable through the self key:

```go
// WithSkip excludes string leaves whose path skip reports true for from rendering.
// Skipped values stay readable through the self key, unrendered.
func WithSkip(skip func(path string) bool) Option
```

Paths use loom's existing format, for example `spec.interfaces[0].dhcp.mac`.

### 5.4 Rendering a decoded tree (`pkg/loom` addition)

`loom.Render` round-trips through YAML. For typed Kubernetes objects that is both lossy
(binary strings) and wasteful (marshal to YAML, parse, re-marshal, convert back).

loom already renders a decoded tree internally (`renderDoc(doc any, ...)`). `pkg/loom`
adds a public function:

```go
// RenderValue renders the templated string leaves of an already-decoded document
// (map[string]any / []any / scalars) in place and returns it.
func RenderValue(doc any, data map[string]any, opts ...Option) (any, error)
```

The backend then renders without any serialization:

```text
typed Hardware
  -> runtime.DefaultUnstructuredConverter.ToUnstructured
  -> loom.RenderValue
  -> runtime.DefaultUnstructuredConverter.FromUnstructured
  -> rendered Hardware (in memory only)
```

The unstructured converter works by reflection, so Go strings, including non-UTF-8
bytes produced by `b64dec`, pass through unchanged. This is what makes binary delivery
work (§5.5). The map and slice types it produces (`map[string]interface{}`,
`[]interface{}`) are the types loom already walks.

### 5.5 Binary contents

Rendered values only ever exist in memory in the backend and its consumers; they are never
written to an API object. With `RenderValue`, a field whose template ends in `b64dec` is
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

**The render store.** The backend keeps a store of rendered Hardware, keyed by Hardware UID
and consumer (§6.5). Each entry records the Hardware `resourceVersion` and the
`resourceVersion` of each resolved reference it was rendered from, the rendered result, and
the last error.

**Background workers.** Event handlers on the shared cache's informers add Hardware keys to
a rate-limited work queue, and a fixed pool of workers renders them:

- Hardware added, updated or deleted.
- A referenced object added, updated or deleted. The store keeps a reverse index from each
  referenced `(group, resource, namespace, name)` to the Hardware that reference it. The
  first time a new referenced GVK appears, the store registers an event handler on that
  GVK's informer (`cache.GetInformer`).
- A referenced Secret's metadata changing (§7.5).

**The request path.** `FilterHardware` does the same indexed lookup as today, then one map
lookup in the store by UID. No template runs and no network call happens.

**Fast path.** When templating is disabled, or a Hardware has no references and no `{{` in
any rendered path, the store records that the rendered result is the stored object and
keeps no copy. Fleets that do not use templating pay nothing beyond the map lookup.

**Referenced data.** Referenced objects are read from the shared cache as unstructured
objects (`client.Options.Cache.Unstructured: true`). The first read of a GVK starts its
informer; later reads are in memory. This replaces the uncached `DynamicRead`. Secrets are
the exception: the store keeps the data of Secrets that at least one Hardware references,
and workers re-fetch a Secret only when its metadata shows a new `resourceVersion`
(§7.5). Memory grows with the referenced Secrets, not with all Secrets.

**Shared references.** When an object referenced by many Hardware changes, all of them are
queued. The queue is rate limited, and requests keep being answered from the previous
renderings until each new one is ready.

**Every replica renders.** Smee and Tootles run in every replica, not only on the leader,
so each replica keeps its own store. Rendering is deterministic, so replicas agree.

**Not ready.** A newly created Hardware that needs rendering has no entry until its first
render completes, typically within milliseconds. Until then, consumers treat it as not
found; DHCP clients retry within seconds.

**Atomic replacement.** Each entry is replaced whole, so a request never sees some fields
from one rendering and some from another.

**Metrics.** Queue depth, render duration, render errors, and the number of entries
currently serving a previous rendering (§5.8).

Informers on referenced types need `list` and `watch` RBAC for those types, not only
`get`. Operators grant them through the chart's existing `rbac.additionalRoleRules`; the
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

- Per field: loom's output cap (§5.3).
- OSIE archive: after rendering for the `smee` consumer, the render store builds the
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
// ResolveReferences returns the Hardware's references that the policy allows for
// consumer, keyed by reference name. Denied or missing references are reported in
// the returned error and omitted from the map.
func (b *Backend) ResolveReferences(ctx context.Context, consumer string, hw *tinkerbell.Hardware) (map[string]any, error)
```

The render store (§5.7) calls this for each consumer it renders for.

### 6.4 Tink Controller

The Workflow reconciler stops evaluating policy and calling `DynamicRead` itself. It builds
its template data from the backend: the rendered Hardware for consumer `tink-controller`,
as a read-only unstructured map, and that consumer's resolved references. A Workflow
Template therefore sees final values under `.Hardware`, matching the v1alpha2 order in
which Hardware is rendered before Workflows. The template engine itself is unchanged in
v1alpha1. With templating disabled, the rendered Hardware is the stored one, so Workflow
rendering behaves exactly as it does today.

### 6.5 Consumer-aware rules

The Quamina event gains a `consumer` field:

```json
{
  "consumer": "smee",
  "source": {"name": "machine1", "namespace": "tinkerbell"},
  "reference": {"name": "machine1-creds", "namespace": "tinkerbell", "group": "", "version": "v1", "resource": "secrets"}
}
```

Consumers have different exposure. A Workflow render is readable only by those who can
read Workflows; Smee serves rendered values to any machine presenting the right MAC
address, and Tootles to any client with the right IP address, both unauthenticated. Rules
can use `consumer` to keep, for example, Secrets out of anything served over the network.

Consumers are `tink-controller`, `smee` and `tootles`. The render store keeps one entry per
Hardware for each consumer enabled in the process, so a Hardware can render successfully
for one consumer and fail for another.

### 6.6 Configuration

The rules keep their existing flags and Helm values, now read by the backend instead of
the Tink Controller:

- `--tink-controller-reference-allow-list-rules` /
  `deployment.envs.tinkController.referenceAllowListRules`
- `--tink-controller-reference-deny-list-rules` /
  `deployment.envs.tinkController.referenceDenyListRules`

They are not renamed. Their names reflect their original user, but a rename would add
flags, Helm keys and a deprecation period for no change in behaviour. The default remains
deny-all: references fail to render until an allow rule is configured, and the Hardware
condition (§8.3) says so.

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
- The cache is shared only when every component would watch the same scope. That is the
  default: `--backend-kube-namespace` is empty and the controllers are cluster-wide. When
  `--backend-kube-namespace` is set, the backend's scope differs from the controllers', and
  each component keeps its own cache as it does today. Sharing in that case would change
  what some component can see.
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
  every enabled consumer:

| Status | Reason | When |
| --- | --- | --- |
| `True` | `Rendered` | Every enabled consumer's rendering succeeded and passed validation |
| `True` | `NoTemplates` | Nothing in the Hardware needs rendering, and it passed validation |
| `False` | `ReferenceDenied` | A reference is denied by policy for a consumer |
| `False` | `ReferenceNotFound` | A referenced object does not exist |
| `False` | `TemplateError` | loom returned a `*FieldError` |
| `False` | `ArchiveTooLarge` | The OSIE archive exceeds `--backend-kube-bootstrap-slot-capacity` (§5.10) |

When `False`, the message names the consumer and the failing field path, and says what is
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
type for consumer `tootles`, with the same not-ready handling as Smee.

## 10. Security

- **Template injection.** Anyone who can edit a Hardware can execute templates in any of
  its `spec` string fields, against that Hardware's resolved references and nothing else.
  Data passed to loom is never re-templated. Functions are the hermetic set only.
- **Confused deputy.** The backend reads referenced objects with its own ServiceAccount on
  behalf of whoever wrote the Hardware. The allow/deny policy, default deny, is the control
  that stops a Hardware author reading objects they could not read directly.
- **Network exposure.** Rendered values are served by Smee to any client presenting the
  Hardware's MAC address, and by Tootles to any client with its IP address. Treat anything
  a `smee` or `tootles` consumer may resolve as disclosed to the provisioning network; use
  `consumer` rules (§6.5) accordingly.
- **Stale data.** After a render failure, the previous rendering, including any Secret
  values it contains, keeps being served (§5.8). Rotating a Secret does not take effect
  for a Hardware whose render is failing.
- **Resource exhaustion.** loom's per-field output cap and time budget bound each render.
  Rendering happens in rate-limited background workers, so request volume never drives
  render volume.
- **RBAC.** Caching referenced types requires `get`/`list`/`watch` on them. Operators
  grant these per type through `rbac.additionalRoleRules`; nothing is granted by wildcard.
  Secret data is never held in the cache (§7.5).

## 11. v1alpha2 alignment

| Concern | v1alpha1 (this design) | v1alpha2 |
| --- | --- | --- |
| Enablement | `--backend-kube-hardware-templating-enabled`, off by default | Always on |
| Templated Hardware fields | All `spec` string fields except §5.2 | Same, with v1alpha2's lookup keys |
| Document and self key | `metadata` + `spec`, `.hardware` | Same |
| Renderer | `pkg/loom` `RenderValue` with `WithSkip` | Same |
| Where rendering runs | Background render store in the kube backend | Same |
| Render failures | Last successful rendering served | Same |
| Policy and resolution | kube backend | kube backend |
| Read type for consumers | rendered `*tinkerbell.Hardware` behind `FilterHardware`-only wrappers | read-only domain type |
| Hardware controller | `Rendered` condition | plus BMC power and inventory |
| Conditions | `metav1.Condition` | `metav1.Condition` (Hardware and Job) |

The v1alpha2 Workflow and Task rendering described in
[v1alpha2 templating](../v1alpha2/templating.md) uses the same loom configuration and
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
2. `pkg/loom`: trimmed copy with `RenderValue` and `WithSkip` (§4.3, §5.3, §5.4).
3. Template functions moved to a shared package (§5.1).
4. Reference policy in the backend: `ResolveReferences`, `consumer`, private
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

Documentation: how to enable templating and escape existing `{{` before doing so, which
fields can be templated, the edit-during-boot note, last-successful fallback and its
behaviour across restarts, consumer rules, and `rbac.additionalRoleRules` for referenced
types.

## 14. Decisions

Questions raised during review, and how they were settled:

| Question | Decision |
| --- | --- |
| Where does loom live? | Copied into `pkg/loom` and maintained in this repository (§4.3). |
| Which fields can be templated? | Every `spec` string field, except `spec.references` and the lookup keys (§5.2). |
| How is templating enabled in v1alpha1? | A process-wide flag, `--backend-kube-hardware-templating-enabled`, off by default (§5.1). |
| How is request latency protected? | Rendering runs in background workers; requests only look up results (§5.7). |
| What happens when a render fails? | The last successful rendering keeps being served, and the `Rendered` condition reports the failure (§5.8). |
| Should Rufio's Secret reads use the cache? | No. Secret data is read live; Secret changes are observed through a metadata-only informer (§7.5). |
| How are reference changes detected? | Informer event handlers registered by the render store at runtime per referenced GVK (§5.7). No periodic resync. |
| How do operators grant access to referenced types? | The chart's existing `rbac.additionalRoleRules`, with `get`, `list` and `watch` (§5.7). |
| How is an oversized OSIE archive reported? | The render store checks it against `--backend-kube-bootstrap-slot-capacity`, default 4 MiB, and reports it through `Rendered`. It does not change what is served (§5.10). |
| Does OSIE delivery get its own condition? | No. `Rendered` covers it; an `OSIEFilesReady` condition would duplicate it. |
| Which condition type? | `metav1.Condition` for v1alpha1 and v1alpha2 Hardware, and v1alpha2 Job (§8.4). |
| Are the reference-rule flags renamed? | No. The backend reads the existing `--tink-controller-reference-*` flags (§6.6). |
| Is `{{` rejected in lookup-key fields? | No. The renderer skips them, so they behave as today (§5.2). |
| Does the Hardware controller run when templating is off? | No (§8.1). |
| Is the cache always shared? | Only when every component watches the same scope, the default (§7.3). |
| Do Smee and Tootles get a new domain type? | Not in v1alpha1. They get `FilterHardware`-only wrappers returning rendered Hardware (§5.6). |

## 15. Testing

- **loom:** `RenderValue` preserves non-UTF-8 bytes; the leaf walker handles the
  unstructured converter's types; a rendered `"0644"` stays a string; `WithSkip` leaves
  skipped values unrendered but readable.
- **Backend:** policy is applied for every consumer; denied and missing references are
  distinct errors; `metadata`, `spec.references` and lookup keys are never rendered; with
  the flag off, every consumer receives the stored object.
- **Render store:** a referenced object's change re-renders exactly the Hardware that
  reference it; a failing render keeps serving the previous one and reports it; a Hardware
  that never rendered is not found; entries are replaced atomically; an oversized OSIE
  archive is reported but the current rendering is still served, including for DHCP and
  iPXE.
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
