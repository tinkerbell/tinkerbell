# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR;

```bash
helm dependency build stack/
trusted_proxies=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
helm install stack stack/ --create-namespace --namespace tink-system --wait --set "boots.boots.trustedProxies=${trusted_proxies}" --set "hegel.hegel.trustedProxies=${trusted_proxies}"
```

## Prerequisites

- Kubernetes 1.23+
- Helm 3.9.4+
