# CaptainOS

[CaptainOS](https://github.com/tinkerbell/captain) is the default Operating System Installation Environment (OSIE) in the Helm chart. HookOS is still available as an alternative.

## Prerequisites

CaptainOS uses Kubernetes [ImageVolumes](https://kubernetes.io/docs/concepts/storage/volumes/#image) to mount its artifacts into the OSIE file server pod. The ImageVolume feature gate must be enabled on your Kubernetes cluster.

| Kubernetes Version | ImageVolume | Default |
|--------------------|-------------|---------|
| 1.31 - 1.32        | Alpha       | false   |
| 1.33 - 1.34        | Beta        | false   |
| 1.35+              | Beta        | true    |

For versions where the default is `false`, you must explicitly enable the `ImageVolume` feature gate on both the kubelet and the API server, or [use HookOS](#using-hookos-instead). If ImageVolume is not enabled, pods using CaptainOS image volumes will fail to be created by the API server.

Reference: <https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/>

## Helm Values

CaptainOS is enabled by default:

```yaml
optional:
  captainos:
    enabled: true
    image: ghcr.io/tinkerbell/captain/artifacts:v0.1.0
    flavor: trixie-full
```

- `optional.captainos.enabled` mounts the CaptainOS artifacts image as a volume in the OSIE deployment.
- `optional.captainos.image` is the CaptainOS artifacts image. The `vX.Y.Z` tag holds the `trixie-full` flavor for x86_64 and aarch64; `vX.Y.Z-slim` holds `trixie-slim`.
- `optional.captainos.flavor` must match the flavor in `image`. It names the kernel and initrd that Smee boots.

When only CaptainOS is enabled and `deployment.envs.smee.ipxeHttpScriptKernelName` and `deployment.envs.smee.ipxeHttpScriptInitrdName` are empty (the default), the chart sets them to `vmlinuz-<flavor>` and `initramfs-<flavor>`. Smee adds the architecture at runtime, so an x86_64 machine boots `vmlinuz-trixie-full-x86_64` and `initramfs-trixie-full-x86_64`. Set those values explicitly to boot other files.

> [!NOTE]
> CaptainOS artifacts have the following naming conventions.
> See the [CaptainOS documentation](https://github.com/tinkerbell/captain) for more details.
> | Artifact | Convention | Example |
> | -------- | ---------- | ------- |
> | kernel | `vmlinuz-<flavor>-<architecture>` | `vmlinuz-trixie-full-x86_64` |
> | initramfs | `initramfs-<flavor>-<architecture>` | `initramfs-trixie-full-x86_64` |
> | ISO | `captainos-<flavor>-<architecture>.iso` | `captainos-trixie-full-x86_64.iso` |

## ISO Boot

The artifacts image includes the CaptainOS ISOs. To serve them through Smee's ISO patching, point `isoUpstreamURL` at the OSIE file server:

```yaml
deployment:
  envs:
    smee:
      isoUpstreamURL: "http://<artifacts file server>:7173/captainos-trixie-full-x86_64.iso"
```

## Using HookOS Instead

```yaml
optional:
  captainos:
    enabled: false
  hookos:
    enabled: true
```

With HookOS enabled, the chart defaults the kernel and initrd names to `vmlinuz` and `initramfs`, as HookOS names them.

## Deploying Together with HookOS

CaptainOS artifacts can be deployed together with HookOS artifacts. When both are enabled, the NGINX file server is configured to serve artifacts from both, with HookOS artifacts tried first and CaptainOS artifacts used as a fallback. Smee boots HookOS by default; select CaptainOS globally with the `ipxeHttpScriptKernelName` and `ipxeHttpScriptInitrdName` values, or per Hardware as shown below.

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
          kernel: "vmlinuz-trixie-full-x86_64"
          initrd: "initramfs-trixie-full-x86_64"
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
