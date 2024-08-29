# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o go-template-file=stack/kubectl.go-template)
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "smee.trustedProxies={${trusted_proxies}}" --set "hegel.trustedProxies={${trusted_proxies}}"
```

## Introduction

This chart boootraps a full Tinkerbell stack on a Kubernetes cluster using the Helm package manager. The Tinkerbell stack consists of the following components:

- [Smee](https://github.com/tinkerbell/smee)
- [Hegel](https://github.com/tinkerbell/hegel)
- [Tink](https://github.com/tinkerbell/tink)
- [Rufio](https://github.com/tinkerbell/rufio)
- [Hook](https://github.com/tinkerbell/hook)
- Reverse proxy server
- DHCP relay agent

This chart also installs a load balancer ([kube-vip](https://kube-vip.io/)) in order to be able to provide a service type loadBalancer IP for the Tinkerbell stack services and an Nginx server for handling proxying to the Tinkerbell service and for serving the Hook artifacts.

## Design details

The stack chart does not use an ingress object and controller. This is because most ingress controllers do not support UDP. Smee uses UDP for DHCP, TFTP, and Syslog services. The ingress controllers that do support UDP require a lot of extra configuration, custom resources, etc. The stack chart deploys a very light weight Nginx deployment with a straightforward configuration that accommodates all the Tinkerbell stack services and serving Hook artifacts.

## Prerequisites

- Kubernetes 1.23+
- Kubectl 1.23+
- Helm 3.9.4+

## Installing the Chart

Before installing the chart you'll want to customize the IP used for the load balancer (`stack.loadBalancerIP`). This IP provides ingress for Hegel, Tink, and Smee (TFTP, HTTP, and SYSLOG endpoints as well as unicast DHCP requests).

Now, deploy the chart.

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o go-template-file=stack/kubectl.go-template)
helm install stack-release stack/ --create-namespace --namespace tink-system --wait --set "smee.trustedProxies={${trusted_proxies}}" --set "hegel.trustedProxies={${trusted_proxies}}"
```

These commands install the Tinkerbell Stack chart in the `tink-system` namespace with the release name of `stack-release`.

## Uninstalling the Chart

To uninstall/delete the `stack-release` deployment:

```bash
helm uninstall stack-release --namespace tink-system
```

## Upgrading the Chart

To upgrade the `stack-release` deployment:

```bash
helm upgrade stack-release stack/ --namespace tink-system --wait
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
| `stack.hook.downloadsDest` | The directory on disk to where Hook artifacts will downloaded  | `/opt/hook` |
| `stack.hook.downloadURL` | The base URL where all Hook tarballs and checksum.txt file exist for downloading | `https://github.com/tinkerbell/hook/releases/download/latest` |

### Load Balancer Parameters (kube-vip)

| Name | Description | Value |
| ---- | ----------- | ----- |
| `stack.kubevip.enabled` | Enable the deployment of the kube-vip load balancer | `true` |
| `stack.kubevip.name` | Name for the kube-vip load balancer service | `kube-vip` |
| `stack.kubevip.image` | Image to use for the kube-vip load balancer | `ghcr.io/kube-vip/kube-vip:v0.5.0` |
| `stack.kubevip.imagePullPolicy` | Image pull policy to use for kube-vip | `IfNotPresent` |
| `stack.kubevip.roleName` | Role name to use for the kube-vip load service | `kube-vip-role` |
| `stack.kubevip.roleBindingName` | Role binding name to use for the kube-vip load service | `kube-vip-rolebinding` |
| `stack.kubevip.interface` | Interface to use for advertizing the load balancer IP. Leaving it unset to allow Kubevip to auto discover the interface to use. | `""` |

### DHCP Relay Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `stack.relay.name` | Name for the relay service | `dhcp-relay` |
| `stack.relay.enabled` | Enable the deployment of the DHCP relay service | `true` |
| `stack.relay.image` | Image to use for the DHCP relay service | `ghcr.io/jacobweinstock/dhcrelay` |
| `stack.relay.maxHopCount` | Maximum number of hops to allow for DHCP relay | `10` |
| `stack.relay.sourceInterface` | Host/Node interface to use for listening for DHCP broadcast packets | `eno1` |
| `stack.relay.presentGiaddrAction` | Control the handling of incoming DHCPv4 packets which already contain relay agent options | `append` |

### Tinkerbell Services Parameters

All dependent services(Smee, Hegel, Rufio, Tink) can have their values overridden here. The following format is used to accomplish this.

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

### Smee Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `smee.hostNetwork` | Whether to deploy Smee using `hostNetwork` on the pod spec. When `true` Smee will be able to receive DHCP broadcast messages. If `false`, Smee will be behind the load balancer VIP and will need to receive DHCP requests via unicast. | `true` |
