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

For IPv6-only installs, replace `--set "publicIP=$LB_IPV4"` with
`--set "publicIPv6=$LB_IPV6"`.

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

For IPv6-only installs, replace `--set "publicIP=$LB_IPV4"` with
`--set "publicIPv6=$LB_IPV6"`.

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

When one or more service IPs are configured, the chart automatically emits the
`kube-vip.io/loadbalancerIPs` Service annotation for kube-vip. To override the
generated value, set the annotation explicitly:

```yaml
service:
  annotations:
    kube-vip.io/loadbalancerIPs: "192.0.2.10,2001:db8::10"
```

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

For DHCPv6, `deployment.envs.smee.dhcpv6ServerDUID` can be set to a stable
server DUID encoded as raw hex bytes with optional `:` or `-` separators. For
example, a DUID-UUID starts with `00:04` followed by the 16 UUID bytes. When the
value is empty, Tinkerbell uses its automatic fallback DUID behavior. In
production Kubernetes deployments, keep this value stable across Pod restarts
and upgrades, for example by sourcing it from a Secret.

## Bind Address Behavior

When `deployment.envs.globals.bindAddr` is set, the Tinkerbell binary binds
shared services to that address. A service-specific bind address, where
available, takes precedence. When the global value is not set, Tinkerbell uses
an IPv4-compatible default selected from addresses detected inside the
container or host:

| Detected IPv4 | Detected IPv6 | Default bind address |
|---------------|---------------|----------------------|
| yes | either | detected IPv4 |
| no | yes | `::` |
| no | no | `0.0.0.0` |

The configured public IPv4 and IPv6 values are advertised addresses and do not
change this default. Set `deployment.envs.globals.bindAddr: "::"` to explicitly
select the IPv6 wildcard for shared services. Whether that listener also accepts
IPv4 traffic is platform dependent; on Linux it depends on `IPV6_V6ONLY` and
`net.ipv6.bindv6only`, and Kubernetes networking may add its own behavior.
DHCPv6 uses its own bind setting, `deployment.envs.smee.dhcpv6BindAddr`, and
defaults to `::`.

For a dual-stack deployment, detecting both address families does not create
dual-stack shared listeners. The automatic bind address remains IPv4 while
`publicIPv6` is still advertised to IPv6 clients. Ensure the advertised IPv6
HTTP, TFTP, syslog, and Tink Server endpoints are reachable by setting an
appropriate IPv6-capable `bindAddr`, configuring service-specific listeners,
or using an IPv6 load balancer or proxy. Setting `dhcpv6BindAddr` alone only
changes the DHCPv6 listener and does not expose the referenced shared services
over IPv6.

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

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.smee.dhcpv6Enabled=true \
  --set deployment.envs.smee.dhcpv6Mode=reservation
```

### Configure Derived DHCPv6 Mode

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.smee.dhcpv6Enabled=true \
  --set deployment.envs.smee.dhcpv6Mode=derived \
  --set deployment.envs.smee.dhcpv6DerivedDirectAddressPool=2001:db8:10::/64 \
  --set deployment.envs.smee.dhcpv6DerivedRelayAddressPrefix=64
```

### Disable DHCPv6 Netboot Options

```bash
helm install tinkerbell . \
  --namespace tinkerbell \
  --set deployment.envs.smee.dhcpv6EnableNetbootOptions=false
```

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
