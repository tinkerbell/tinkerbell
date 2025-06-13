# Tinkerbell Helm Chart

This Helm chart deploys Tinkerbell, the bare metal provisioning engine that supports network and ISO booting, BMC interactions, metadata service, and a workflow engine.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+

## Installation

### Quick Start

```bash
# Get the pod CIDRs to set as trusted proxies
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')

# Set the LoadBalancer IP for Tinkerbell services
LB_IP=192.168.2.116

# Set the artifacts file server URL for HookOS
ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

helm template tinkerbell . \
  --create-namespace \
  --namespace tinkerbell \
  --wait \
  --set "trustedProxies={${trusted_proxies}}" \
  --set "publicIP=$LB_IP" \
  --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" \
  --set "deployment.agentImageTag=latest" \
  --set "deployment.imageTag=latest"
```

> [!NOTE]  
> The `--set "deployment.agentImageTag=latest"` and `--set "deployment.imageTag=latest"` are only needed when doing a `helm install` from the file location.

### Production Installation

For a production setup, configure the necessary parameters:

```bash
# Get the pod CIDRs to set as trusted proxies
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')

# Set the LoadBalancer IP for Tinkerbell services
LB_IP=192.168.2.116

# Set the artifacts file server URL for HookOS
ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

# Specify the Tinkerbell Helm chart version, here we use the latest release.
TINKERBELL_CHART_VERSION=$(basename $(curl -Ls -o /dev/null -w %{url_effective} https://github.com/tinkerbell/tinkerbell/releases/latest))

helm install tinkerbell oci://ghcr.io/tinkerbell/charts/tinkerbell \
  --version $TINKERBELL_CHART_VERSION \
  --create-namespace \
  --namespace tinkerbell \
  --wait \
  --set "trustedProxies={${trusted_proxies}}" \
  --set "publicIP=$LB_IP" \
  --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" 
```

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

## Required Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `publicIP` | Public IP for Tinkerbell services | `""` |
| `trustedProxies` | List of trusted proxy CIDRs | `[]` |
| `artifactsFileServer` | URL for the HookOS artifacts server | `""` |

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

## Additional Resources

- [Tinkerbell Documentation](https://tinkerbell.org)
- [GitHub Repository](https://github.com/tinkerbell/tinkerbell)
- [Community Slack](https://cloud-native.slack.com/archives/C01SRB41GMT)

## License

This project is licensed under the Apache License 2.0 - see the [`LICENSE`](../../LICENSE ) file for details.
