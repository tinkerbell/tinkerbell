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
