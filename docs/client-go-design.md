# Proposal: publish supported client-go libraries for Tinkerbell APIs

## Summary

Tinkerbell should publish a nested Go module at `client-go/` with module path
`github.com/tinkerbell/tinkerbell/client-go`. The module would provide the
conventional Kubernetes clientset, fake clientset, listers, and shared informer
factory for Tinkerbell custom resources while preserving the existing
`github.com/tinkerbell/tinkerbell/api/...` import paths.

This document is written as an issue draft. The initial pull request should be
limited to the accompanying `tinkerbell.org/v1alpha1` Hardware proof of concept
until maintainers agree on the module placement and code-generation approach.

## Motivation

Consumers currently have to maintain handwritten REST clients, scheme setup,
fake clients, listers, and informers for Tinkerbell resources. In particular,
`uds-remote-agent` carries this plumbing for Hardware, Template, Workflow, BMC
Job, and BMC Machine. A supported upstream module would remove duplicated code,
keep resource behavior aligned with the CRDs, and give downstream controllers a
normal client-go API.

The API module should remain lightweight. It intentionally does not depend on
`k8s.io/client-go`, and this proposal does not change that boundary.

## Proposed module and releases

- Directory: `client-go/`
- Module: `github.com/tinkerbell/tinkerbell/client-go`
- Tags: `client-go/vX.Y.Z`
- Preferred cadence: version and tag it in lockstep with `api/vX.Y.Z`
- Kubernetes dependencies: align `k8s.io/client-go`, `k8s.io/code-generator`,
  `k8s.io/apimachinery`, and related modules with the API and root modules
- Initial supported API: all v1alpha1 root resources
- v1alpha2: either include it in the first supported release or track it in an
  explicit follow-up; it first needs a Tinkerbell `AddToScheme` helper

The release script and release documentation currently know about the root and
`api` tags only. They would need to create and document the third tag.

## API inventory

The inventory below comes from both the Go root-object markers and the generated
CRDs under `crd/bases`. Every currently generated resource is namespaced.

| Group/version | Kind | Resource | Served | Storage | Status |
| --- | --- | --- | --- | --- | --- |
| `tinkerbell.org/v1alpha1` | Hardware | `hardware` | yes | yes | yes |
| `tinkerbell.org/v1alpha1` | Template | `templates` | yes | yes | yes |
| `tinkerbell.org/v1alpha1` | Workflow | `workflows` | yes | yes | yes |
| `tinkerbell.org/v1alpha1` | WorkflowRuleSet | `workflowrulesets` | yes | yes | yes |
| `bmc.tinkerbell.org/v1alpha1` | Job | `jobs` | yes | yes | yes |
| `bmc.tinkerbell.org/v1alpha1` | Machine | `machines` | yes | yes | yes |
| `bmc.tinkerbell.org/v1alpha1` | Task | `tasks` | yes | yes | yes |
| `tinkerbell.org/v1alpha2` | Hardware | `hardware` | yes | yes | yes |
| `tinkerbell.org/v1alpha2` | Policy | `policies` | yes | yes | no |
| `tinkerbell.org/v1alpha2` | Task | `tasks` | yes | yes | yes |
| `tinkerbell.org/v1alpha2` | Workflow | `workflows` | yes | yes | yes |
| `bmc.tinkerbell.org/v1alpha2` | Job | `jobs` | yes | yes | yes |

The v1alpha1 Tinkerbell and BMC packages and the v1alpha2 BMC package provide
`AddToScheme`. The v1alpha2 Tinkerbell package currently provides only
`GroupVersion`, so it cannot yet participate in an aggregate client scheme.

## Package-layout decision

Kubernetes's `kube_codegen.sh` expects API input in `group/version` layout.
Tinkerbell's stable public API imports use `version/group`, for example
`api/v1alpha1/tinkerbell`. The client module must not break those paths.

There are three viable approaches:

1. Add codegen-facing `group/version` adapter packages whose exported object and
   list types alias the canonical API types. Use this only if a spike proves
   `client-gen`, `lister-gen`, and `informer-gen` correctly discover aliases,
   preserve the original runtime types, and produce complete status methods.
2. Move canonical definitions to `group/version` and leave aliases at all old
   paths. This is the cleanest generator layout but is the most invasive option
   and should happen only with explicit maintainer approval and compatibility
   tests.
3. Start with a handwritten, `gentype`-based clientset. This preserves the API
   module unchanged and closely matches modern generated client code, at the
   cost of maintaining a small deterministic source template or generator in
   this repository.

The accompanying proof of concept uses option 3. It establishes REST and
consumer behavior without pre-deciding a broad API package relocation.

## MVP behavior

The supported module should eventually include every root resource in the
selected versions, not just resources used by one downstream project. It should
provide:

- aggregate scheme registration
- `Interface`, `Clientset`, `Discovery`, `NewForConfig`,
  `NewForConfigAndClient`, and `NewForConfigOrDie`
- group/version typed clients with JSON negotiation
- Get, List, Watch, Create, Update, Delete, DeleteCollection, and Patch
- UpdateStatus and status patch paths only for CRDs declaring `/status`
- fake clients with action and selector behavior
- typed namespace-aware listers
- shared informer factory options for namespace and `TweakListOptions`
- `ForResource`, `Start`, `Shutdown`, and `WaitForCacheSync`

Apply configurations can follow separately if their generated footprint makes
the initial contribution substantially larger.

## Validation and integration

The complete implementation should test scheme registration, REST paths and
serialization, all verbs and query parameters, patch content types and status
paths, shared HTTP transport, fake CRUD/watch/selectors/actions, informer
list/watch/index/sync behavior, listers, discovery, deterministic regeneration,
`go mod tidy`, race tests, lint, and the complete repository CI workflow.

Once the upstream shape is accepted, a temporary `uds-remote-agent` branch can
replace its handwritten clientset and informer factory with this module. That
proof should preserve its existing shared lifecycle and cache behavior and
report deleted downstream code and remaining API gaps back to the upstream pull
request. Lease locking, ownership policy, feature flags, and workflow-state
semantics remain downstream concerns.

## Maintainer decisions requested

1. Is `client-go/` the desired module location and
   `github.com/tinkerbell/tinkerbell/client-go` the desired module path?
2. Should `client-go/vX.Y.Z` be released in lockstep with `api/vX.Y.Z`?
3. Which package-layout/code-generation option should be used after the
   one-resource proof?
4. Is all of v1alpha1 the first supported surface, and should v1alpha2 ship in
   the same release or an explicitly tracked follow-up?

Broad generation and any API package movement should wait for agreement on
questions 1 and 3.
