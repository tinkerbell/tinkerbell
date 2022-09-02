# Tinkerbell Stack

This chart installs the full Tinkerbell stack.

## TL;DR;

```bash
helm dependency build stack/
helm install stack stack/ --create-namespace --namespace tink-system --wait
```

## Prerequisites

- Kubernetes 1.23+
- Helm 3.9.4+

