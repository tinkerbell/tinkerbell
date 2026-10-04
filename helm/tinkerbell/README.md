# Tinkerbell Helm Chart

This Helm chart deploys Tinkerbell, the bare metal provisioning engine that supports network and ISO booting, BMC interactions, metadata service, and a workflow engine.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+

## Installation

### Quick Start

```bash
# Get the pod CIDRs to set as trusted proxies
TRUSTED_PROXIES=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')

# Set the IPv4 LoadBalancer IP for Tinkerbell services
LB_IPV4=192.168.2.116
# For IPv6-only services, set publicIPv6 instead of publicIP.
# LB_IPV6=2001:db8:100::116

# Set the artifacts file server URL for HookOS
ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

# Set the IPv6 artifacts file server URL for HookOS when needed
# ARTIFACTS_FILE_SERVER_V6=http://[2001:db8:100::102]:7173

helm upgrade --install tinkerbell . \
  --create-namespace \
  --namespace tinkerbell \
  --wait \
  --set "trustedProxies={${TRUSTED_PROXIES}}" \
  --set "publicIP=$LB_IPV4" \
  --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" \
  --set "artifactsFileServerV6=$ARTIFACTS_FILE_SERVER_V6" \
  --set "deployment.agentImageTag=latest" \
  --set "deployment.imageTag=latest"
```

For IPv6-only installs, use `publicIPv6` and `artifactsFileServerV6` instead of
their IPv4 counterparts. Configure the [Service IP families](#service-ip-families)
and [bind addresses](#bind-address-behavior) for IPv6 reachability.

> [!NOTE]  
> The `--set "deployment.agentImageTag=latest"` and `--set "deployment.imageTag=latest"` are only needed when doing a `helm install` from the file location.

### Production Installation

For a production setup, configure the necessary parameters:

```bash
# Get the pod CIDRs to set as trusted proxies
TRUSTED_PROXIES=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')

# Set the IPv4 LoadBalancer IP for Tinkerbell services
LB_IPV4=192.168.2.116
# For IPv6-only services, set publicIPv6 instead of publicIP.
# LB_IPV6=2001:db8:100::116

# Set the artifacts file server URL for HookOS
ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

# Set the IPv6 artifacts file server URL for HookOS when needed
# ARTIFACTS_FILE_SERVER_V6=http://[2001:db8:100::102]:7173

# Specify the Tinkerbell Helm chart version, here we use the latest release.
TINKERBELL_CHART_VERSION=$(basename $(curl -Ls -o /dev/null -w %{url_effective} https://github.com/tinkerbell/tinkerbell/releases/latest))

helm install tinkerbell oci://ghcr.io/tinkerbell/charts/tinkerbell \
  --version $TINKERBELL_CHART_VERSION \
  --create-namespace \
  --namespace tinkerbell \
  --wait \
  --set "trustedProxies={${TRUSTED_PROXIES}}" \
  --set "publicIP=$LB_IPV4" \
  --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" \
  --set "artifactsFileServerV6=$ARTIFACTS_FILE_SERVER_V6"
```

For IPv6-only installs, use `publicIPv6` and `artifactsFileServerV6` instead of
their IPv4 counterparts. Configure the [Service IP families](#service-ip-families)
and [bind addresses](#bind-address-behavior) for IPv6 reachability.

### Optional Components

#### HookOS

The HookOS section provides downloading and file serving of HookOS artifacts.

```yaml
optional:
  hookos:
    enabled: true
    kernelVersion: both  # 5.10, 6.6, both
    arch: both  # x86_64, aarch64, both
    downloadURL: https://github.com/tinkerbell/hook/releases/download/v0.10.0
```

#### Kube-vip

The Kube-vip section provides a Kubernetes LoadBalancer implementation. A LoadBalancer IP is required for Tinkerbell services.

```yaml
optional:
  kubevip:
    enabled: true
    image: ghcr.io/kube-vip/kube-vip:v0.9.1
```

For both the main and OSIE Services, when the Service type is `LoadBalancer`
and one or more load balancer IPs are configured, the chart automatically emits
the `kube-vip.io/loadbalancerIPs` annotation for kube-vip. This applies to
IPv4-only, IPv6-only, and dual-stack configurations. To override a generated
value, set the annotation explicitly in `service.annotations` or
`optional.osie.service.annotations`:

```yaml
service:
  annotations:
    kube-vip.io/loadbalancerIPs: "192.0.2.10,2001:db8::10"
```

## Service IP Families

The main Tinkerbell Service and the OSIE artifact Service have independent
IP-family settings. Configure both when boot clients need to reach Tinkerbell
and download kernel/initramfs artifacts over the same families. OSIE does not
inherit the main Service's settings.

| Value | Main Service | OSIE artifact Service |
|-------|--------------|-----------------------|
| Family policy | `service.ipFamilyPolicy` | `optional.osie.service.ipFamilyPolicy` |
| Family selection and order | `service.ipFamilies` | `optional.osie.service.ipFamilies` |

The defaults are `ipFamilyPolicy: ""` and `ipFamilies: []`. Helm omits both
fields, preserving Kubernetes' default behavior for new Services:

| Kubernetes cluster | Service created with the defaults |
|--------------------|-----------------------------------|
| IPv4-only | Single-stack IPv4 |
| IPv6-only | Single-stack IPv6 |
| Dual-stack | Single-stack, using the cluster's primary Service IP family |

**A dual-stack cluster does not make these Services dual-stack automatically.**
Setting both `publicIP` and `publicIPv6`, both artifact URLs, or two addresses
in the kube-vip annotation does not change the Service's family policy.

### Adapt to the cluster

To use both families where the cluster supports them and fall back to one on
single-stack clusters, set `PreferDualStack` on both Services. Leave
`ipFamilies` unset so Kubernetes selects the available families and their order:

```yaml
service:
  ipFamilyPolicy: PreferDualStack
optional:
  osie:
    service:
      ipFamilyPolicy: PreferDualStack
```

### Require dual-stack

Use `RequireDualStack` when both families are mandatory. Kubernetes rejects
the Service if the cluster does not support dual-stack. This example selects
IPv4 as the primary family; omit `ipFamilies` to keep the cluster's order:

```yaml
service:
  ipFamilyPolicy: RequireDualStack
  ipFamilies: [IPv4, IPv6]
optional:
  osie:
    service:
      ipFamilyPolicy: RequireDualStack
      ipFamilies: [IPv4, IPv6]
```

### Select IPv6 only

On an IPv6-only cluster, the empty defaults already select IPv6. To explicitly
select IPv6 on a dual-stack cluster, including one whose primary family is
IPv4, configure both Services as follows:

```yaml
service:
  ipFamilyPolicy: SingleStack
  ipFamilies: [IPv6]
optional:
  osie:
    service:
      ipFamilyPolicy: SingleStack
      ipFamilies: [IPv6]
```

Use `[IPv4]` instead to explicitly select IPv4. The selected family must be
supported by the cluster. When upgrading an existing Service, retain its
primary family: Kubernetes allows adding or removing a secondary family but
does not allow changing the primary family in place. See the
[Kubernetes dual-stack Service documentation](https://kubernetes.io/docs/concepts/services-networking/dual-stack/#services).

These examples configure Service IP allocation. Also configure the matching
public addresses and artifact URLs in [Required Values](#required-values),
the families the pod listens on as described in [Listener Families](#listener-families),
the port values described in [Per-Family Ports](#per-family-ports),
and cluster networking and a load balancer that support the requested families.
Service family settings do not reach Smee; whether DHCPv6 runs is decided by
`deployment.envs.globals.listenerFamilies` and
`deployment.envs.smee.dhcpv6Enabled`. Those two settings also decide whether the
DHCP and DHCPv6 Service ports are published at all, so the Service never
advertises a port the pod is not listening on. The OSIE Service settings
apply only when the chart creates that Service; no OSIE Service is created
when its artifact server uses host networking.

## Required Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `publicIP` | IPv4 address advertised to IPv4 boot clients | `""` |
| `publicIPv6` | IPv6 address advertised to IPv6 boot clients | `""` |
| `trustedProxies` | List of trusted proxy CIDRs | `[]` |
| `artifactsFileServer` | URL for the HookOS artifacts server | `""` |
| `artifactsFileServerV6` | IPv6 URL for the HookOS artifacts server | `""` |

For IPv6 boot flows, set the IPv6-specific values such as `publicIPv6`,
`artifactsFileServerV6`, and `deployment.envs.smee.ipxeScriptTinkServerAddrPortV6`
as needed. Do not rely on a dual-stack DNS name in the IPv4/common values to
make IPv6 clients work; the IPv6 iPXE path uses the IPv6-specific values.
`publicIP` and `publicIPv6` must contain addresses of their named family;
IPv4-mapped IPv6 values such as `::ffff:192.0.2.10` are IPv4 and are not valid
for `publicIPv6`.

`deployment.envs.smee.ipxeScriptSyslogFqdnV6` sets the syslog hostname or IPv6
address for DHCPv6 boot scripts (`auto6.ipxe`). When empty, it uses
`deployment.envs.smee.dhcpv6SyslogIP`. IPv4 scripts continue to use
`ipxeScriptSyslogFqdn`, falling back to `dhcpSyslogIP`. iPXE resolves the selected
hostname on the booting client each time the script runs, so DNS changes take
effect without regenerating the script. Resolution failures emit a warning and
do not prevent booting.

Use an A-only hostname for IPv4 and an AAAA-only hostname for IPv6. The bundled
iPXE resolver prefers AAAA when IPv6 DNS is configured and cannot explicitly
request an address family. An answer from the wrong family is rejected without
changing syslog settings.

For DHCPv6, `deployment.envs.smee.dhcpv6ServerDUID` can be set to a stable
server DUID encoded as raw hex bytes with optional `:` or `-` separators. For
example, a DUID-UUID starts with `00:04` followed by the 16 UUID bytes. When the
value is empty, Tinkerbell uses its automatic fallback DUID behavior. In
production Kubernetes deployments, keep this value stable across Pod restarts
and upgrades, for example by sourcing it from a Secret.

## Listener Families

`deployment.envs.globals.listenerFamilies` selects the IP address families every
listener serves.

| Value | Listeners |
|-------|-----------|
| `ipv4` (default) | IPv4 only |
| `ipv6` | IPv6 only |
| `dual` | both |

**Values for a family that is not served are ignored, not rejected.** One values
file can therefore configure both families, and switching between IPv4-only,
IPv6-only, and dual-stack is a one-line change. Tinkerbell logs the ignored
settings at startup, for example:

```text
"msg":"ignoring flags for address families not being served",
"listenerFamilies":"ipv4","ignoredFlags":["bind-address-v6","public-ip-v6"]
```

This governs the *family* of every listener, including DHCP. It does not turn
services on: each service keeps its own enable value, and the two combine. DHCP
is enabled for both families by default, so `dual` answers DHCPv4 and DHCPv6,
while `ipv6` answers only DHCPv6. Set `deployment.envs.smee.dhcpv6Enabled:
false` to serve IPv6 HTTP, TFTP, syslog, gRPC, and SSH without running a DHCPv6
server, for example where the network already provides one.

Public address auto-detection follows the same rule: a family that is not served
is never detected and never advertised.

This is independent of `service.ipFamilies`, which controls the families
Kubernetes assigns to the Service. Set both; see
[Service IP Families](#service-ip-families).

For the underlying flags and how to verify the result, see
[IP Family Configuration](../../docs/technical/IP_FAMILY_CONFIGURATION.md).

## Bind Address Behavior

Within the families being served, shared services bind one socket per family.
The IPv6 socket is always `IPV6_V6ONLY`, so an IPv6 wildcard never accepts IPv4
traffic and the two families can share a port number.

`deployment.envs.globals.bindAddr` sets the IPv4 bind address and
`deployment.envs.globals.bindAddrV6` sets the IPv6 one. A service-specific bind
address, where available, takes precedence. When a served family has no
configured address, Tinkerbell selects a default:

| Family | Detected address | Default bind address |
|--------|------------------|----------------------|
| IPv4 | yes | the detected IPv4 address |
| IPv4 | no | `0.0.0.0` |
| IPv6 | either | `::` |

The configured public IPv4 and IPv6 values are advertised addresses, possibly
belonging to a load balancer, and do not change these defaults. DHCPv6 uses its
own bind setting, `deployment.envs.smee.dhcpv6BindAddr`, and defaults to `::`.

Setting `dhcpv6BindAddr` alone only changes the DHCPv6 listener; use
`listenerFamilies` to expose the shared services over IPv6.

## Per-Family Ports

Each bind port has an IPv6 counterpart. Leave the IPv6 value empty and it
inherits the IPv4 value, which is the recommended setting:

| IPv4 value | IPv6 value |
|------------|------------|
| `deployment.envs.globals.httpPort` | `deployment.envs.globals.httpPortV6` |
| `deployment.envs.globals.httpsPort` | `deployment.envs.globals.httpsPortV6` |
| `deployment.envs.tinkServer.bindPort` | `deployment.envs.tinkServer.bindPortV6` |
| `deployment.envs.secondstar.bindPort` | `deployment.envs.secondstar.bindPortV6` |
| `deployment.envs.smee.tftpServerBindPort` | `deployment.envs.smee.tftpServerBindPortV6` |
| `deployment.envs.smee.syslogBindPort` | `deployment.envs.smee.syslogBindPortV6` |

**A Service port entry has a single `targetPort` that both families share.**
Kubernetes provides no way to route IPv4 and IPv6 to different container ports
through one Service, so when `listenerFamilies` is `dual` and `service.enabled`
is `true`, each IPv6 value must equal its IPv4 counterpart. The chart fails at
render time rather than creating a listener the Service cannot reach:

```text
deployment.envs.smee.tftpServerBindPortV6 (6969) must equal
deployment.envs.smee.tftpServerBindPort (69): a Service port has a single
targetPort shared by both IP families.
```

Only one family is served in the `ipv4` and `ipv6` modes, so there is no shared
`targetPort` to reconcile and the values may differ. The values belonging to a
family that is not served are ignored, exactly as Tinkerbell ignores them, and
the container ports the pod declares follow whichever family is served.

To bind a different port per family while serving both, set
`service.enabled: false` and reach the pod directly, for example with
`deployment.hostNetwork: true`. Note that `hostNetwork` alone is not enough:
traffic arriving through a Service still lands on `targetPort`.

DHCP and DHCPv6 are unaffected: they have separate Service entries, so
`dhcpBindPort` and `dhcpv6BindPort` may differ. Each declares the container port
it binds, and each entry is published only when its family is served.

## Additional RBAC Rules

The `rbac.additionalRoleRules` field allows appending custom RBAC policy rules to the Tinkerbell role. Each entry follows the Kubernetes [PolicyRule](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole) schema. There are two mutually exclusive rule types:

**Resource-based rules** (for Kubernetes API resources like configmaps, pods, etc.):

- Required: `apiGroups`, `resources`, `verbs` (all must be arrays of strings)
- Optional: `resourceNames`

**Non-resource URL rules** (for non-resource endpoints like /healthz, /metrics):

- Required: `nonResourceURLs`, `verbs` (both must be arrays of strings)
- `apiGroups`, `resources`, and `resourceNames` must **not** be specified

Resource-based rule:

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set-json 'rbac.additionalRoleRules=[{"apiGroups":[""],"resources":["configmaps"],"verbs":["get","list"]}]'
```

Using `resourceNames` to restrict access to specific resources:

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set-json 'rbac.additionalRoleRules=[{"apiGroups":[""],"resources":["configmaps"],"resourceNames":["my-config"],"verbs":["get"]}]'
```

Non-resource URL rule:

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set-json 'rbac.additionalRoleRules=[{"nonResourceURLs":["/healthz","/metrics"],"verbs":["get"]}]'
```

> [!CAUTION]
> When `rbac.type` is `ClusterRole` (the default), additional rules grant **cluster-wide** access, not just within the release namespace.
> No content-level validation is performed on rule values. Users are responsible for following the principle of least privilege.
> Avoid wildcards (`*`) and privileged verbs (`escalate`, `bind`, `impersonate`) unless absolutely necessary.

When `rbac.type` is `Role`, permissions apply only in the Helm release namespace. Set
`deployment.envs.globals.backendKubeNamespace` to that namespace; use `ClusterRole` for
cluster-wide watching and cluster-scoped CRD migrations. With a non-empty backend namespace,
Hardware references are limited to namespaced objects there, so a `Role` can grant access
using `rbac.additionalRoleRules`. With an empty backend namespace, references can span
namespaces and require a `ClusterRole`. In either mode, add rules granting `get`, `list`, and
`watch` on each referenced API resource.

## Examples

### Disabling specific services

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.globals.enableSmee=false \
  --set deployment.envs.globals.enableTinkServer=false \
  --set deployment.envs.globals.enableTinkController=false
```

### Enable Auto-Enrollment

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.tinkServer.autoEnrollmentEnabled=true
```

### Configure DHCP Mode

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.smee.dhcpMode=auto-proxy
```

### Configure DHCPv6 Mode

DHCPv6 runs only when the listener families include IPv6.

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.globals.listenerFamilies=dual \
  --set deployment.envs.smee.dhcpv6Mode=reservation
```

### Configure Derived DHCPv6 Mode

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.globals.listenerFamilies=dual \
  --set deployment.envs.smee.dhcpv6Mode=derived \
  --set deployment.envs.smee.dhcpv6DerivedDirectAddressPool=2001:db8:10::/64 \
  --set deployment.envs.smee.dhcpv6DerivedRelayAddressPrefix=64
```

### Serve IPv6 Without Running DHCPv6

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.globals.listenerFamilies=dual \
  --set deployment.envs.smee.dhcpv6Enabled=false
```

### Disable DHCPv6 Netboot Options

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.smee.dhcpv6EnableNetbootOptions=false
```

## Helm Namespace and RBAC Configuration

### Helm Chart Defaults

| Helm value | Default |
|---|---|
| `deployment.envs.globals.backend` | `"kube"` |
| `deployment.envs.globals.backendKubeNamespace` | `""` |
| `deployment.envs.globals.backendKubeRenderingEnabled` | `false` |
| `deployment.envs.globals.enableCRDMigrations` | `true` |
| `deployment.envs.globals.enableTinkController` | `true` |
| `deployment.envs.globals.enableRufioController` | `true` |
| `deployment.envs.tinkServer.autoDiscoveryEnabled` | `false` |
| `deployment.envs.tinkServer.autoDiscoveryNamespace` | `""` |
| `deployment.envs.tinkController.referenceAllowListRules` | `[]` |
| `deployment.envs.tinkController.referenceDenyListRules` | `[]` |
| `rbac.type` | `ClusterRole` |
| `rbac.additionalRoleRules` | `[]` |
| `rbac.secrets.enabled` | `true` |

Assume only the RBAC resources installed by this chart; additional RoleBindings or ClusterRoleBindings can grant more API permissions, but they do not change which namespaces Tinkerbell watches.

The **Helm release namespace** is the namespace where Helm installs Tinkerbell. `backendKubeNamespace` sets the **backend namespace**: when empty, the backend and the Tink Controller and Rufio caches watch all namespaces; when set, they watch only that namespace. The backend is Tinkerbell’s shared Kubernetes access layer, used by Smee, Tootles, Tink Server, and SecondStar. The optional render store is the backend’s background Hardware renderer; it starts only when Hardware templating is enabled. `autoDiscoveryNamespace` sets the **auto-discovery namespace**. Helm resolves an empty value to the Helm release namespace. When auto-discovery is enabled, Tink Server creates discovered Hardware there.

Hardware references are objects named in `Hardware.spec.references`. The allow-list and deny-list values control Tinkerbell’s policy for resolving references, not Kubernetes permissions. With both lists empty, the resolver denies references by default. For templated Hardware, the render store watches every in-scope declared reference type whether or not policy allows the reference, so Kubernetes RBAC must permit `list` and `watch` on those resource types within the backend's watched namespace scope. Tinkerbell reads an individual referenced object only when policy allows it, so `get` permission is needed only for references allowed by policy. The chart grants Secret access separately by default; other referenced resource types need `rbac.additionalRoleRules`.

CRD migrations run at startup and create or update Kubernetes CustomResourceDefinition objects. CRDs are cluster-scoped resources, and migrations are enabled by default. A namespaced `Role` cannot authorize these operations; with `Role`, disable migrations only if the CRDs are already installed.

### `rbac.type: Role`

Helm creates the `Role` and `RoleBinding` in the Helm release namespace. This Role authorizes namespaced API requests only in that namespace. `rbac.additionalRoleRules` adds permissions there; it cannot grant access in other namespaces or to cluster-scoped resources.

The chart defaults `rbac.type` to `ClusterRole`; none of the following `Role` combinations is the chart default.

Examples below use `tinkerbell` as the Helm release namespace and `machines` and `discovery` as other namespaces. Replace them with the namespaces in your installation.

| Backend namespace | Auto-discovery namespace | Support status | Expected behavior | Example Helm overrides |
|---|---|---|---|---|
| Helm release namespace | Helm release namespace | Supported for namespaced operations if CRDs are installed and migrations are disabled. | Components can list, watch, and reconcile resources in the release namespace. If auto-discovery is enabled, Hardware is created there and is visible to them. | `rbac.type: Role`<br>`deployment.envs.globals.backendKubeNamespace: tinkerbell`<br>`deployment.envs.globals.enableCRDMigrations: false` |
| Empty (Helm default) | Empty (Helm default) | Not supported with the chart’s Role alone. | All-namespace cache LIST/WATCH requests receive Kubernetes `Forbidden` errors because the Role authorizes only the release namespace. The controller caches cannot sync, so those controllers do not reconcile. | `rbac.type: Role`<br>`deployment.envs.globals.enableCRDMigrations: false` |
| A namespace other than the Helm release namespace | Any namespace | Not supported with the chart’s Role alone. | Cache LIST/WATCH requests in the configured backend namespace receive `Forbidden` errors; the controller caches cannot sync or reconcile there. | `rbac.type: Role`<br>`deployment.envs.globals.backendKubeNamespace: machines`<br>`deployment.envs.globals.enableCRDMigrations: false` |
| Helm release namespace | A different namespace | Not supported when auto-discovery is enabled. | The Hardware create request receives `Forbidden`, and the discovery operation returns an error. With auto-discovery disabled, the auto-discovery namespace has no effect. | `rbac.type: Role`<br>`deployment.envs.globals.backendKubeNamespace: tinkerbell`<br>`deployment.envs.globals.enableCRDMigrations: false`<br>`deployment.envs.tinkServer.autoDiscoveryEnabled: true`<br>`deployment.envs.tinkServer.autoDiscoveryNamespace: discovery` |

When the backend namespace is set to the Helm release namespace, Hardware references must point to namespaced objects in that namespace. Cross-namespace and cluster-scoped references are rejected, even if an allow-list rule matches. Use `rbac.additionalRoleRules` to grant `get`, `list`, and `watch` on referenced resource types there; Tinkerbell’s reference allow/deny rules must also permit them. The default CRD migrations must be disabled after the CRDs are installed.

### `rbac.type: ClusterRole`

Helm creates a `ClusterRole` and `ClusterRoleBinding`, authorizing namespaced requests in any namespace and cluster-scoped operations. This authorization does not widen caches: the configured backend namespace still controls which namespaces the backend, controllers, and render store watch.

| Backend namespace | Auto-discovery namespace | Support status | Expected behavior | Example Helm overrides |
|---|---|---|---|---|
| Empty (Helm default) | Empty (Helm default) | **Helm chart default:** `ClusterRole`, with auto-discovery disabled. | Components watch all namespaces. Tink Server does not create Hardware unless auto-discovery is enabled. If enabled, Hardware is created in the release namespace and is visible to the caches. | No overrides |
| A configured namespace | The same namespace | Supported. | Components watch only the configured backend namespace. If auto-discovery is enabled, the Hardware create succeeds and the object is visible to backend reads and controllers. | `deployment.envs.globals.backendKubeNamespace: machines`<br>`deployment.envs.tinkServer.autoDiscoveryNamespace: machines`<br>`deployment.envs.tinkServer.autoDiscoveryEnabled: true` |
| A configured namespace | A different namespace | Misconfigured when auto-discovery is enabled. | The Hardware create can succeed, but the object is outside the components’ watched namespace and is not visible to their cache-backed reads or controllers. A later discovery can miss it and fail a duplicate create with `AlreadyExists`. With auto-discovery disabled, the auto-discovery namespace has no effect. | `deployment.envs.globals.backendKubeNamespace: machines`<br>`deployment.envs.tinkServer.autoDiscoveryNamespace: discovery`<br>`deployment.envs.tinkServer.autoDiscoveryEnabled: true` |

When the backend namespace is empty, references may point to namespaced objects in any namespace or to cluster-scoped objects, subject to the reference allow/deny rules. Add `rbac.additionalRoleRules` granting `get`, `list`, and `watch` on every referenced API resource; these rules apply cluster-wide. When a backend namespace is configured, references are limited to namespaced objects in that namespace. Both the reference policy and Kubernetes RBAC must permit each reference. The chart’s default `ClusterRole` also authorizes the default cluster-scoped CRD migrations.

## Upgrading from Helm chart version 0.6.2

> [!IMPORTANT]
> Before upgrading ensure there are no actively running `workflows.tinkerbell.org` or `jobs.bmc.tinkerbell.org`.
> Once confirmed, changing the replica count to 0 for all Tinkerbell components will ensure no further reconciliation or processing occurs during the upgrade.

The CRDs from v0.6.2 have been updated in v0.19.x. There is no action required for users upgrading from v0.6.2 to v0.19.x.

- **No breaking changes** in the Custom Resource Definitions (CRDs)
- Additional status fields have been added to the Workflow CRD
- CRDs will be automatically updated when deploying the v0.19.x Helm chart

> [!Note]
> To disable automatic CRD migrations, use the flag `--set "deployment.envs.globals.enableCRDMigrations=false"` during deployment. If disabled, you must manually update CRDs (not covered in this guide).

For help migrating your `values.yaml` from 0.6.2 to 0.19.x, please refer to the [migration guide](../../docs/technical/HELM_VALUES_MIGRATION.md).

## Additional Resources

- [Tinkerbell Documentation](https://tinkerbell.org)
- [GitHub Repository](https://github.com/tinkerbell/tinkerbell)
- [Community Slack](https://cloud-native.slack.com/archives/C01SRB41GMT)

## License

This project is licensed under the Apache License 2.0 - see the [`LICENSE`](../../LICENSE ) file for details.
