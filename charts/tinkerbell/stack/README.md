# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "boots.trustedProxies=${trusted_proxies}" --set "hegel.trustedProxies=${trusted_proxies}"
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

Before installing the chart you'll want to customize the IP used for the load balancer (`stack.loadBalancerIP`). This IP provides ingress for Hegel, Tink, and Boots (TFTP, HTTP, and SYSLOG endpoints as well as unicast DHCP requests).

You'll also want to set the IP used in DHCP packets for option 54, the location of the iPXE binaries, the `auto.ipxe` script, the syslog IP, and the IP for downloading Hook files (`boots.remoteIp`).

The vast majority of the time,these 2 (`stack.loadBalancerIP` and `boots.remoteIp`) IPs will be the same.

Now, deploy the chart.

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "boots.trustedProxies=${trusted_proxies}" --set "hegel.trustedProxies=${trusted_proxies}"
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
| `stack.name` | Name for the stack chart | `tink-stack` |
| `stack.service.type` | Type of service to use for the Tinkerbell stack services. One of the [standard](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) Kubernetes service types. | `LoadBalancer` |
| `stack.selector` | Selector(s) to use for the mapping stack deployment with the service | `app: tink-stack` |
| `stack.loadBalancerIP` | Load balancer IP address to use for the Tinkerbell stack services | `192.168.2.111` |
| `stack.lbClass` | loadBalancerClass to use for in stack service | `kube-vip.io/kube-vip-class` |
| `stack.image` | Image to use for the proxying to Tinkerbell services and serving artifacts | `nginx:1.23.1` |
| `stack.hook.enabled` | Enable the deployment of the Hook artifacts | `true` |
| `stack.hook.name` | Name for the Hook artifacts server | `hook-files` |
| `stack.hook.port` | Port to use for the Hook artifacts server | `8080` |
| `stack.hook.image` | Image to use for downloading the Hook artifacts | `alpine` |
| `stack.hook.downloads` | List of Hook artifacts to download | `[]` |
| `stack.hook.downloads[0].url` | URL of the Hook bundle to download | `""` |
| `stack.hook.downloads[0].sha512sum` | sha512sum of the Hook bundle | `""` |

### Load Balancer Parameters (kube-vip)

| Name | Description | Value |
| ---- | ----------- | ----- |
| `kubevip.enabled` | Enable the deployment of the kube-vip load balancer | `true` |
| `kubevip.name` | Name for the kube-vip load balancer service | `kube-vip` |
| `kubevip.image` | Image to use for the kube-vip load balancer | `ghcr.io/kube-vip/kube-vip:v0.5.0` |
| `kubevip.imagePullPolicy` | Image pull policy to use for kube-vip | `IfNotPresent` |
| `kubevip.roleName` | Role name to use for the kube-vip load service | `kube-vip-role` |
| `kubevip.roleBindingName` | Role binding name to use for the kube-vip load service | `kube-vip-rolebinding` |
| `kubevip.interface` | Interface to use for advertizing the load balancer IP. Leaving it unset to allow Kubevip to auto discover the interface to use. | `""` |

### DHCP Relay Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `stack.relay.name` | Name for the relay service | `dhcp-relay` |
| `stack.relay.enabled` | Enable the deployment of the DHCP relay service | `true` |
| `stack.relay.image` | Image to use for the DHCP relay service | `ghcr.io/jacobweinstock/dhcrelay` |
| `stack.relay.maxHopCount` | Maximum number of hops to allow for DHCP relay | `10` |
| `stack.relay.sourceInterface` | Host/Node interface to use for listening for DHCP broadcast packets | `eno1` |

### Tinkerbell Services Parameters

All dependent services(Boots, Hegel, Rufio, Tink) can have their values overridden here. The following format is used to accomplish this.

```yaml
<service name>:
  <key to override>: <value>
  <array key to override>:
    - <key>: <value>
```

Example:

```yaml
hegel:
  image: quay.io/tinkerbell/hegel:latest
```

### Boots Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `boots.hostNetwork` | Whether to deploy Boots using `hostNetwork` on the pod spec. When `true` Boots will be able to receive DHCP broadcast messages. If `false`, Boots will be behind the load balancer VIP and will need to receive DHCP requests via unicast. | `true` |
