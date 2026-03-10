# CaptainOS

[CaptainOS](https://github.com/tinkerbell/captain) is an alternative Operating System Installation Environment (OSIE) that can be used alongside or instead of HookOS.

## Prerequisites

CaptainOS uses Kubernetes [ImageVolumes](https://kubernetes.io/docs/concepts/storage/volumes/#image) to mount its artifacts into the OSIE file server pod. The ImageVolume feature gate must be enabled on your Kubernetes cluster.

| Kubernetes Version | ImageVolume | Default |
|--------------------|-------------|---------|
| 1.31 - 1.32        | Alpha       | false   |
| 1.33 - 1.34        | Beta        | false   |
| 1.35+              | Beta        | true    |

For versions where the default is `false`, you must explicitly enable the `ImageVolume` feature gate on both the kubelet and the API server. If ImageVolume is not enabled, pods using CaptainOS image volumes will fail to be created by the API server.

Reference: <https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/>

## Enabling CaptainOS

CaptainOS is optional and disabled by default. To enable it, set the following Helm values:

```yaml
optional:
  captainos:
    enabled: true

deployment:
  envs:
    smee:
      ipxeHttpScriptKernelName: "vmlinuz-6.18.16"
      ipxeHttpScriptInitrdName: "initramfs-6.18.16"
```

Or via `--set` flags:

```bash
helm install tinkerbell tinkerbell/ \
  --set optional.captainos.enabled=true \
  --set deployment.envs.smee.ipxeHttpScriptKernelName=vmlinuz-6.18.16 \
  --set deployment.envs.smee.ipxeHttpScriptInitrdName=initramfs-6.18.16
```

> [!NOTE]
> The `ipxeHttpScriptKernelName` and `ipxeHttpScriptInitrdName` values should not include an architecture suffix.
> The architecture suffix is added at runtime by Tinkerbell when generating the iPXE script, based on the architecture of the machine being provisioned. For example, if `ipxeHttpScriptKernelName` is set to `vmlinuz` and the machine is x86_64, the generated iPXE script will reference `vmlinuz-x86_64`.

- `optional.captainos.enabled` enables the CaptainOS artifacts image volume in the OSIE deployment.
- `deployment.envs.smee.ipxeHttpScriptKernelName` sets the kernel filename used in the iPXE boot script (default: `vmlinuz`). Architecture is added at runtime.
- `deployment.envs.smee.ipxeHttpScriptInitrdName` sets the initrd filename used in the iPXE boot script (default: `initramfs`). Architecture is added at runtime.

> [!NOTE]
> CaptainOS artifacts have the following naming conventions.
> See the [CaptainOS documentation](https://github.com/tinkerbell/captain) for more details.
> | Artifact | Convention | Example |
> | -------- | ---------- | ------- |
> | kernel | `vmlinuz-<kernel version>-<architecture>` | `vmlinuz-6.18.16-x86_64` |
> | initramfs | `initramfs-<kernel version>-<architecture>` | `initramfs-6.18.16-x86_64` |
> | ISO | `captainos-<kernel version>-<architecture>.iso` | `captainos-6.18.16-x86_64.iso` |

## Deploying Together with HookOS

CaptainOS artifacts can be deployed together with HookOS artifacts. When both are enabled, the NGINX file server is configured to serve artifacts from both, with HookOS artifacts tried first and CaptainOS artifacts used as a fallback.

```yaml
optional:
  captainos:
    enabled: true
  hookos:
    enabled: true
```

## Per-Hardware Kernel and Initrd Configuration

The kernel and initrd filenames set in the Helm values apply globally. To override these on a per-Hardware basis, use the `osie` fields in the Hardware spec under `spec.interfaces[].netboot.osie`:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: my-machine
  namespace: tinkerbell
spec:
  interfaces:
    - netboot:
        allowPXE: true
        osie:
          baseURL: "http://192.168.2.117:7173"
          kernel: "vmlinuz-6.18.16-custom"
          initrd: "initramfs-6.18.16-custom"
      dhcp:
        ip:
          address: 192.168.2.10
          gateway: 192.168.2.1
          netmask: 255.255.255.0
        mac: "00:1a:2b:3c:4d:5e"
```

- `osie.baseURL` overrides the base URL from which the kernel and initrd are downloaded.
- `osie.kernel` overrides the kernel filename for this specific Hardware.
- `osie.initrd` overrides the initrd filename for this specific Hardware.

When set on a Hardware resource, these values take precedence over the global Helm values for that machine.
