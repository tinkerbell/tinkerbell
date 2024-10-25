# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o go-template-file=stack/kubectl.go-template)
LB_IP=<IP_ADDRESS>
helm install stack-release stack/ --create-namespace --namespace tink --wait --set "global.trustedProxies={${trusted_proxies}}" --set "global.publicIP=$LB_IP"
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

This chart also installs a load balancer ([kube-vip](https://kube-vip.io/)) in order to be able to provide a service type loadBalancer IP for the Nginx server that handles proxying to all the Tinkerbell services and for serving the Hook artifacts. This kube-vip load balancer is the default but can be disabled if another load balancer is preferred.

## Design details

The stack chart does not use an ingress object and controller. This is because most ingress controllers do not support multi-protocol (UDP, TCP, and gRPC in the Tinkerbell case). Smee uses UDP for DHCP, TFTP, and Syslog services. The ingress controllers that do support UDP require a lot of extra configuration, custom resources, etc. The Tinkerbell stack (Hegel and Smee) also needs the source IP or X-ForwardFor enabled to provide the appropriate data to clients. This is not generally available in an ingress controller. As such, the stack chart deploys a very light weight Nginx deployment with a straightforward configuration that accommodates all the Tinkerbell stack services and serving Hook artifacts. As [Gateway API](https://gateway-api.sigs.k8s.io/) (and the implementations of it) matures there is hope that it will be possible to use it to deploy the Tinkerbell stack instead.

## Prerequisites

- Kubernetes 1.23+
- Kubectl 1.23+
- Helm 3.9.4+

## Installing the Chart

Before installing the chart you'll want to customize the IP used for the load balancer (`global.publicIP`). This IP provides ingress for Hegel, Tink, and Smee (TFTP, HTTP, and SYSLOG endpoints as well as unicast DHCP requests). You'll also need to provide the trusted proxies for the Tinkerbell services. The trusted proxies are the IP addresses of the nodes in the cluster. The trusted proxies are used to set the `X-Forwarded-For` header in the Nginx configuration. This is necessary for the Tinkerbell services to get the correct client IP address. The `kubectl.go-template` file is provided to get the IP addresses of the nodes in the cluster. `kubectl get nodes -o go-template-file=stack/kubectl.go-template`

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

### JSON Schema

Helm has the ability to validate a `values.yaml` file via a JSON schema (`values.schema.json`). See the [Helm documentation](https://helm.sh/docs/topics/charts/#schema-files) for more details. Each chart in the Tinkerbell stack has a very basic schema file that is used to validate either RBAC and/or trusted proxies. Each schema file is located next to the `values.yaml` file for each respective chart.

### RBAC

All Tinkerbell services need RBAC permissions to run in a Kubernetes cluster. There are two options for the type of RBAC that can be used. `Role` or `ClusterRole`. Each chart has its own way to toggle between the two. Most of them are located in their `values.yaml` file under `rbac.type`. There is also a global value in the Stack chart that can be used to set the RBAC type for all services. The global value is `global.rbac.type`. The default for all services and for the global value is `Role`.

### Persistence

The only persistence needed for the Tinkerbell stack is if you are downloading HookOS. By default HookOS is downloaded and stored in a local Persistent Volume. If you're cluster has a different storage class you want ot use, can override the default persistent volume claim by setting the `stack.hook.persistence.existingClaim` value.
