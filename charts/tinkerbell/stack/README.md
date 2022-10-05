# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "boots.boots.trustedProxies=${trusted_proxies}" --set "hegel.hegel.trustedProxies=${trusted_proxies}"
```

## Introduction

This chart bootstraps a full Tinkerbell stack on a Kubernetes cluster using the Helm package manager. The Tinkerbell stack consists of the following components:

- [Boots](https://github.com/tinkerbell/boots)
- [Hegel](https://github.com/tinkerbell/hegel)
- [Tink](https://github.com/tinkerbell/tink)
- [Rufio](https://github.com/tinkerbell/rufio)

This chart also installs a load balancer ([kube-vip](https://kube-vip.io/)) in order to be able to provide a service type loadBalancer IP for the Tinkerbell stack services and an Nginx server for handling proxying to the Tinkerbell service and for serving the Hook artifacts.

## Design details

The stack chart does not use an ingress object and controller. This is because most ingress controllers do not support UDP. Boots uses UDP for DHCP, TFTP, and Syslog services. The ingress controllers that do support UDP require a lot of extra configuration, custom resources, etc. The stack chart deploys a very light weight Nginx deployment with a straightforward configuration that accommodates all the Tinkerbell stack services and serving Hook artifacts.

## Prerequisites

- Kubernetes 1.23+
- Kubectl 1.23+
- Helm 3.9.4+

## Installing the Chart

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "boots.boots.trustedProxies=${trusted_proxies}" --set "hegel.hegel.trustedProxies=${trusted_proxies}"
```

These commands install the Tinkerbell Stack chart in the `tink-system` namespace with the release name of `stack-release`.

## Uninstalling the Chart

To uninstall/delete the `stack-release` deployment:

```bash
helm uninstall stack-release --namespace tink-system
```

## Parameters

### Stack Service Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `stack.enabled` | Enable the deployment of the Tinkerbell stack chart | `true` |
| `stack.name` | Name of the Tinkerbell stack chart | `tink-stack` |
| `stack.service.type` | Type of service to use for the Tinkerbell stack services | `LoadBalancer` |
| `stack.selector.app` | Selector to use for the Tinkerbell stack services | `tink-stack` |
| `stack.selector.lbtype` | Selector to use for the Tinkerbell stack services | `external` |
| `stack.ip` | IP address to use for the Tinkerbell stack services | `192.168.2.111` |
| `stack.lbClass` | Class to use for the Tinkerbell stack services | `kube-vip.io/kube-vip-class` |
| `stack.image` | Image to use for the Tinkerbell stack services | `nginx:1.23.1` |
| `stack.hook.enabled` | Enable the deployment of the Hook artifacts server | `true` |
| `stack.hook.name` | Name of the Hook artifacts server | `hook-files` |
| `stack.hook.port` | Port to use for the Hook artifacts server | `8080` |
| `stack.hook.image` | Image to use for downloading the Hook artifacts | `alpine` |
| `stack.hook.downloads` | List of Hook artifacts to download | `[]` |
| `stack.hook.downloads[0].url` | URL of the Hook artifact to download | `""` |
| `stack.hook.downloads[0].sha512sum.kernel` | Name of the Hook artifact to download | `""` |
| `stack.hook.downloads[0].sha512sum.initramfs` | Name of the Hook artifact to download | `""` |

### Load Balancer Parameters (kube-vip)

| Name | Description | Value |
| ---- | ----------- | ----- |
| `kubevip.enabled` | Enable the deployment of the kube-vip load balancer | `true` |
| `kubevip.name` | Name of the kube-vip load balancer | `kube-vip` |
| `kubevip.image` | Image to use for the kube-vip load balancer | `ghcr.io/kube-vip/kube-vip:v0.5.0` |
| `kubevip.imagePullPolicy` | Image pull policy to use for the kube-vip load balancer | `IfNotPresent` |
| `kubevip.roleName` | Role name to use for the kube-vip load balancer | `kube-vip-role` |
| `kubevip.roleBindingName` | Role binding name to use for the kube-vip load balancer | `kube-vip-rolebinding` |
| `kubevip.interface` | Interface to use for advertizing the load balancer IP | `eth0` |

### Tinkerbell Services Parameters

All dependent services(Boots, Hegel, Rufio, Tink) can have their values overridden here. The following format is used to accomplish this.

```yaml
<service name>:
  <service name>:
    <key to override>: <value>
    <array key to override>:
      - <key>: <value>
```

Example:

```yaml
hegel:
  hegel:
    image: quay.io/tinkerbell/hegel:latest
```
