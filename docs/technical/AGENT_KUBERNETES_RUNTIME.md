# Kubernetes `RuntimeExecutor` for `tink-agent`

This document describes the `kubernetes` `RuntimeExecutor`, a way to run `tink-agent` as a
standing pod inside a Kubernetes cluster that executes Actions as Kubernetes `Job`s instead of
using a Docker or containerd socket.

## When to use this

`tink-agent` normally runs on the machine being provisioned, using the `docker` or `containerd`
runtime to execute Actions directly on that hardware (for example `image2disk`, which must run on
the specific machine it's imaging). The `kubernetes` runtime is **not** a replacement for that: a
`Job`'s Pod is scheduled by Kubernetes wherever it decides, with no relationship to any particular
piece of bare metal.

Instead, this runtime is for a different kind of Agent: one that runs as a long-lived pod inside a
Kubernetes cluster (typically the cluster running the Tink Controller) to execute Actions that
don't need to touch specific target hardware — for example a Workflow Task, on a standing
`agentID`, that calls a webhook to notify an external system before or after the actual
provisioning Tasks run. That standing Agent still needs some way to run Action containers. Giving
it a Docker or containerd socket means either mounting the host node's socket (root-equivalent
node access from a pod inside your own cluster) or running Docker-in-Docker as a sidecar
(`privileged: true` either way). The `kubernetes` runtime avoids both: the Agent's pod only needs
RBAC to create/watch `Job`s and `Pod`s and read `Pod` logs in one namespace.

## Enabling it

```bash
tink-agent -id=abcd -transport=grpc -grpc-server=tink-server.tinkerbell.svc.cluster.local:42113 \
  -runtime=kubernetes \
  -kubernetes-namespace=tinkerbell \
  -kubernetes-service-account=tink-agent
```

| Flag | Default | Description |
|---|---|---|
| `-kubernetes-namespace` | `tinkerbell` | Namespace Action `Job`s are created in. |
| `-kubernetes-kubeconfig` | *(empty)* | Path to a kubeconfig file. Leave empty to use the in-cluster config — the expected setup when the Agent itself runs as a pod in the cluster it's creating `Job`s in. |
| `-kubernetes-service-account` | *(empty)* | ServiceAccount the Action `Job`'s pod runs as. Kubernetes does **not** propagate the Agent's own ServiceAccount to objects it creates, so leaving this empty means the `Job` pod runs as the namespace's `default` ServiceAccount, not the Agent's. Set this to the Agent's own ServiceAccount name (e.g. `tink-agent`) to reuse the imagePullSecrets configured on it. |

## Timeouts

An Action's `timeoutSeconds` bounds both the Agent's own wait (as with every other runtime) and,
independently, the Job's `activeDeadlineSeconds`. The latter is a backstop enforced by the cluster
itself (kubelet/kube-controller-manager): if the Agent pod is OOM-killed, crashes, or restarts
mid-Action, the Job would otherwise be orphaned with nothing left to enforce the timeout or clean
it up.

## Unsupported Action fields

Because a `Job`'s Pod can land on any node, some Action fields have no safe Kubernetes equivalent
and are rejected outright (the Action fails fast with a clear error) rather than being silently
ignored:

- `namespaces.pid` / `namespaces.network` (`hostPID`/`hostNetwork`) — cluster-wide elevated
  privileges that would defeat the purpose of using this runtime instead of a Docker socket.
- `volumes` — Docker bind-mount syntax (`/etc/data:/data:ro`) has no faithful Kubernetes
  equivalent short of `hostPath`.

## RBAC

The Agent only needs namespaced permissions — no `secrets`, no cluster-scoped resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tink-agent-kubernetes-runtime
  namespace: tinkerbell
rules:
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch", "delete"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tink-agent-kubernetes-runtime
  namespace: tinkerbell
subjects:
  - kind: ServiceAccount
    name: tink-agent
    namespace: tinkerbell
roleRef:
  kind: Role
  name: tink-agent-kubernetes-runtime
  apiGroup: rbac.authorization.k8s.io
```

Private images are pulled using whatever `imagePullSecrets` are already configured on the
`tink-agent` ServiceAccount — but only if `-kubernetes-service-account=tink-agent` is set (see
[Enabling it](#enabling-it)); otherwise the Job pod runs as the namespace's `default`
ServiceAccount instead and won't see these secrets. This is a one-time, cluster-operator-managed
setup:

```bash
kubectl create secret docker-registry my-registry-cred \
  --docker-server=... --docker-username=... --docker-password=... \
  --namespace tinkerbell
kubectl patch serviceaccount tink-agent -n tinkerbell \
  -p '{"imagePullSecrets": [{"name": "my-registry-cred"}]}'
```

The `kubernetes` runtime itself never creates or reads Secrets, so no `secrets` RBAC verb is
needed.

## Example Deployment

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tink-agent
  namespace: tinkerbell
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tink-agent-standalone
  namespace: tinkerbell
spec:
  replicas: 1
  selector:
    matchLabels: {app: tink-agent-standalone}
  template:
    metadata:
      labels: {app: tink-agent-standalone}
    spec:
      serviceAccountName: tink-agent
      containers:
        - name: tink-agent
          image: ghcr.io/tinkerbell/tink-agent:latest
          args:
            - -id=abcd
            - -transport=grpc
            - -grpc-server=tink-server.tinkerbell.svc.cluster.local:42113
            - -runtime=kubernetes
            - -kubernetes-namespace=tinkerbell
            - -kubernetes-service-account=tink-agent
```

No `privileged`, no `hostNetwork`, no sidecar, no host socket mount — the whole pod is a single,
unprivileged container.
