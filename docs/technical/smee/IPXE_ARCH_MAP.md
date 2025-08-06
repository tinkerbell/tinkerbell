# iPXE Architecture Mapping

This document outlines the built-in mapping of iPXE binaries to specific architectures and how to specify overrides.

## Background

In Tinkerbell, when a machine boots over the network, it relies on DHCP to provide the location of an iPXE binary. The iPXE binary is architecture-specific, meaning that different architectures require different binaries to boot correctly.
Generally, the processor architecture type is a combination of the CPU architecture of the machine (x86, ARM, etc.), the machine's firmware type (UEFI, BIOS, etc.), and the boot method (HTTP, PXE, etc.). The processor architecture type of a machine can be determined by inspecting DHCP option 93 (Client System Architecture) ([source](https://www.rfc-editor.org/rfc/rfc4578.html#section-2.1) and [errata](https://www.rfc-editor.org/errata_search.php?rfc=4578)) of the DHCP packet send by the machine during the network boot process.

## Supported iPXE Binaries

Tinkerbell builds, embeds, and serves (via TFTP and HTTP(s)) its own iPXE binaries for different architectures. The following iPXE binaries are supported:

- `undionly.kpxe`: A universal iPXE binary for BIOS systems.
- `ipxe.efi`: An iPXE binary for UEFI systems.
- `snp-arm64.efi`: An iPXE binary for ARM64 UEFI systems using the Simple Network Protocol(SNP).
- `snp-x86_64.efi`: An iPXE binary for x86_64 UEFI systems using the Simple Network Protocol(SNP).

## Built-in Mappings

Smee provides a set of built-in mappings for common architectures. These mappings are defined in the [`ArchToBootFile`](../../../smee/internal/dhcp/dhcp.go) function. They map an IANA Processor Architecture Type (references: [Go library](https://github.com/insomniacslk/dhcp/tree/master/iana) and [iana](https://www.iana.org/assignments/dhcpv6-parameters/dhcpv6-parameters.xhtml#processor-architecture)) to a specific iPXE binary.

> [!Note]
> These are global and are used across all machines that network boot.

**Built-in Mappings Table:**

| IANA Architecture | uint16 | iPXE Binary |
|-------------------|:------:|-------------|
| Intel x86PC                | 0               | undionly.kpxe |
| NEC/PC98                   | 1               | undionly.kpxe |
| EFI Itanium                | 2               | undionly.kpxe |
| DEC Alpha                  | 3               | undionly.kpxe |
| Arc x86                    | 4               | undionly.kpxe |
| Intel Lean Client          | 5               | undionly.kpxe |
| EFI IA32                   | 6               | ipxe.efi |
| EFI x86-64                 | 7               | ipxe.efi |
| EFI Xscale                 | 8               | ipxe.efi |
| EFI BC                     | 9               | ipxe.efi |
| EFI ARM32                  | 10              | snp-arm64.efi |
| EFI ARM64                  | 11              | snp-arm64.efi |
| EFI x86 boot from HTTP     | 15              | ipxe.efi |
| EFI x86-64 boot from HTTP  | 16              | ipxe.efi |
| EFI ARM32 boot from HTTP   | 18              | snp-arm64.efi |
| EFI ARM64 boot from HTTP   | 19              | snp-arm64.efi |
| Intel x86PC boot from HTTP | 20              | undionly.kpxe |
| arm rpiboot                | 41              | snp-arm64.efi |

## Global override of the Built-in Mapping

To override the global built-in mapping, there is a CLI flag (`--ipxe-override-arch-mapping`) and an environment variable (`TINKERBELL_IPXE_OVERRIDE_ARCH_MAPPING`).
Any of the built-in mappings can be overridden and new additional mappings can be added. The format for the overrides is: `<iana arch uint16>=<ipxe binary>`.
Additional overrides can be specified by separating them with a comma.

For example:

- override the `EFI x86-64 boot from HTTP` mapping to use the `snp-x86_64.efi` binary instead of the `ipxe.efi` binary

  ```bash
  # CLI Flag
  --ipxe-override-arch-mapping="8=snp-x86_64.efi"
  # Environment variable
  export TINKERBELL_IPXE_OVERRIDE_ARCH_MAPPING="8=snp-x86_64.efi"
  ```

- override multiple mappings at once

  ```bash
  # CLI Flag
  --ipxe-override-arch-mapping="7=snp-x86_64.efi,8=snp-arm64.efi"
  # Environment variable
  export TINKERBELL_IPXE_OVERRIDE_ARCH_MAPPING="7=snp-x86_64.efi,8=snp-arm64.efi"
  ```

## Hardware specific overrides

To override the iPXE binary for a specific machine, you can specify the iPXE binary in the machine's corresponding Hardware object at `spec.interfaces[].netboot.ipxe.binary`. The mac address of the network interface is used to match the Hardware object with the DHCP request.

For example, setting `spec.interfaces[0].netboot.ipxe.binary` to `snp-x86_64.efi` in the Hardware object below will override the iPXE binary for the machine with the MAC address `de:ad:be:ef:00:01` to use `snp-x86_64.efi` instead of the default binary for its architecture.

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: example
spec:
  interfaces:
    - dhcp:
        arch: x86_64
        hostname: example
        ip:
          address: 192.168.2.148
          gateway: 192.168.2.1
          netmask: 255.255.255.0
        lease_time: 86400
        mac: de:ad:be:ef:00:01
        name_servers:
          - 1.1.1.1
          - 8.8.8.8
      netboot:
        allowPXE: true
        ipxe:
          binary: snp-x86_64.efi
```
