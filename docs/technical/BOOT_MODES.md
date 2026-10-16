# Workflow Boot Modes

This document describes the different boot modes available in the Workflow `spec.bootOptions.bootMode`.

## netboot

The following is an example Workflow with the boot mode set to `netboot`.

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Workflow
metadata:
  name: example-workflow
spec:
  bootOptions:
    bootMode: netboot
```

The `netboot` mode gets a Machine into a network (PXE) booting state. This is accomplished by the Tink Controller automatically creating a `job.bmc.tinkerbell.org` during the `PREPARING` Workflow state. The `job.bmc.tinkerbell.org` is created with the following specification:

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Job
metadata:
  name: netboot-workflow-name
spec:
  machineRef:
    name: example
    namespace: example
  tasks:
    - powerAction: "off"
    - oneTimeBootDeviceAction:
        device:
          - "pxe"
        efiBoot: false # or true based on Hardware.spec.interfaces[].dhcp.uefi
    - powerAction: "on"
```

[Reference](/tink/controller/internal/workflow/pre.go#L49-L72)

## isoboot

The following is an example Workflow with the boot mode set to `isoboot`. When `isoboot` is specified, the `spec.bootMode.isoURL` is required.
This URL should point to the Tinkerbell IP and the port defined in the Helm chart values. The port is defined at `service.ports.http.port`, which defaults to 7080.
The MAC Address in the `spec.bootMode.isoURL` should match the Hardware that this Workflow references (`spec.hardwareRef`). Also, the MAC Address should be `-` (minus sign) delimited.
This is a limitation observed in most BMCs. Experiment with your Hardware to find exceptions.

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Workflow
metadata:
  name: example-workflow
spec:
  bootOptions:
    bootMode: isoboot
    isoURL: http://<tinkerbell VIP>:7080/iso/02-7f-92-bd-2d-57/hook.iso
```

The `isoboot` mode gets a Machine booted into HookOS by mounting the HookOS ISO, served by the Smee service, as a virtual CD-ROM and then ejecting the virtual CD-ROM after the Workflow. This is accomplished by the Tink Controller by automatically creating a `job.bmc.tinkerbell.org` during the `PREPARING` and `POST` Workflow states. During the `PREPARING` Workflow state the following `job.bmc.tinkerbell.org` to mount the HookOS ISO is created:

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Job
metadata:
  name: isoboot-workflow-name
spec:
  machineRef:
    name: example
    namespace: example
  tasks:
    - powerAction: "off"
    - virtualMediaAction:
        mediaURL: "http://tinkerbell-ip:7080/iso/:macAddress/hook.iso"
        kind: "CD"
    - oneTimeBootDeviceAction:
        device:
          - "cdrom"
        efiBoot: false # or true based on Hardware.spec.interfaces[].dhcp.uefi
    - powerAction: "on"
```

[Reference](/tink/controller/internal/workflow/pre.go#L106-L141)

During the `POST` Workflow state, the following `job.bmc.tinkerbell.org` is created to eject the HookOS ISO:

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Job
metadata:
  name: isoboot-workflow-name
spec:
  machineRef:
    name: example
    namespace: example
  tasks:
    - virtualMediaAction:
        mediaURL: ""
        kind: "CD"
```

[Reference](/tink/controller/internal/workflow/post.go#L35-L42)

## customboot

The `customboot` mode lets the customizations of the `job.bmc.tinkerbell.org` used in the `PREPARING` and `POST` Workflow states. This allows defining anything from rebooting the Machine to setting the next boot device. The following is an example of defining the `customboot` mode with `preparingActions` and `postActions`:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example
spec:
  templateRef: example
  hardwareRef: example
  hardwareMap:
    worker_id: de:ad:be:ef:00:01
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - bootDevice:
          device: "pxe"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - powerAction: "reset"
```

[PREPARING Reference](/tink/controller/internal/workflow/pre.go#L158)
[POST Reference](/tink/controller/internal/workflow/post.go#L61)

#### Example: PXE Boot with customboot

The `customboot` mode can replicate the `netboot` behavior while allowing additional customization. This is useful when you need fine-grained control over the boot sequence:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-pxe-boot
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - bootDevice:
          device: "pxe"
          efiBoot: true  # Set based on your hardware requirements
      - powerAction: "on"
      postActions:
      - powerAction: "off"
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - powerAction: "on"
```

This configuration will:
1. Power off the machine
2. Set the next boot device to PXE
3. Power on the machine (boots from network)
4. After workflow completion, power off the machine
5. Set boot device back to disk persistently
6. Power on the machine to boot from disk

#### Example: Enabling UEFI HTTP Boot with customboot

The `networkBootConfig` action enables/disables UEFI HTTP Boot and/or legacy PXE boot capability in BIOS/UEFI firmware, and can set the URL UEFI HTTP Boot fetches its boot image from. Unlike `bootDevice` (which selects among existing boot options), `networkBootConfig` creates or removes the boot options themselves - useful when a Machine's firmware doesn't have UEFI HTTP Boot enabled by default.

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-http-boot
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - networkBootConfig:
          httpBootEnabled: true
          httpBootURL: 'http://172.17.1.1:7080/ipxe/binary/ipxe.efi'
      - powerAction: "on"
```

`httpBootURL` points at the iPXE binary Tinkerbell serves under `/ipxe/binary/` (`ipxe.efi` for x86_64, `snp-arm64.efi` for arm64). It must be an `http://` or `https://` URL, and it is not templated - unlike `virtualMediaAction.mediaURL`, `customboot`'s Go-template support (e.g. `{{ (index .Hardware.Interfaces 0).DHCP.MAC }}`) does not apply to `networkBootConfig` fields, so the URL must not contain template syntax.

`httpBootEnabled`, `pxeBootEnabled`, `httpBootURL`, and `httpBootTLSMode` are independent - any combination may be set at once, and a field left unset leaves that setting untouched. Support for this action depends on the BMC's provider implementing it in [bmclib](https://github.com/bmc-toolbox/bmclib); `httpBootTLSMode` is currently only implemented for Dell (iDRAC/Redfish) and returns an error on other providers.

#### Example: UEFI Secure Boot around a PXE boot

HookOS and iPXE are typically unsigned, so a Machine with Secure Boot enforced can't network boot at all. The `secureBoot` action's `enable` and `resetKeys` operations let a Workflow disable Secure Boot and delete the PK before netbooting, then re-enable Secure Boot once a trusted key has been enrolled into the installed OS's `db` (for example by a Template Action that writes the `db` EFI variable while the firmware is in Setup Mode):

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-secureboot
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - secureBoot:
          enable: false
      - secureBoot:
          resetKeys:
            resetType: DeletePK
      - bootDevice:
          device: "pxe"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - secureBoot:
          enable: true
      - powerAction: "off"
      - powerAction: "on"
```

This configuration will:
1. Power off the machine
2. Disable Secure Boot, since the netboot chain below isn't signed
3. Delete the PK, which puts the firmware in Setup Mode - the state a Template Action needs to enroll new keys
4. Set the next boot device to PXE and power on
5. After the Template has installed the OS and enrolled a trusted key, set the boot device back to disk persistently
6. Re-enable Secure Boot, then power off and on so the firmware applies it on the next boot (a power cycle may not apply staged changes on every BMC)

The `resetType` of `resetKeys` is one of:

| `resetType` | Effect |
|---|---|
| `ResetAllKeysToDefault` | Restores every key database to its platform default. The firmware ends up with a PK, so it is not in Setup Mode. |
| `DeleteAllKeys` | Empties every key database, PK included. The firmware enters Setup Mode. |
| `DeletePK` | Empties only the PK database. The firmware enters Setup Mode, but `db`, `KEK` and `dbx` keep their entries, so keys that were trusted before stay trusted. |

In Setup Mode the firmware doesn't enforce Secure Boot, so `enable: true` only takes effect once a PK has been enrolled.

> [!WARNING]
> `postActions` only run when the Workflow succeeds. If the Workflow fails or times out after `enable: false`, the Machine is left with Secure Boot disabled and no PK. Re-enable it with a Job or Task that runs `secureBoot: {enable: true}`.

#### Example: Enrolling a trust anchor with a certificate import

`secureBoot.certificateImport` adds a certificate to a single key database (`db`, for example) directly through the BMC, without entering Setup Mode or running a Template Action inside the booted OS to write EFI variables. The boot chain can then be signed with that certificate, so Secure Boot enforcement can stay enabled throughout:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-secureboot-import
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - secureBoot:
          certificateImport:
            database: db
            certificatePEMConfigMapRef:
              name: provisioning-trust-anchor
              key: cert.pem
      - bootDevice:
          device: "pxe"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - powerAction: "cycle"
```

This configuration will:
1. Power off the machine
2. Import the certificate in the `cert.pem` key of the `provisioning-trust-anchor` ConfigMap into `db`
3. Set the next boot device to PXE and power on
4. After provisioning, set the boot device back to disk persistently and power-cycle

Before using `certificateImport`:
- The ConfigMap must exist in the same namespace as the Workflow, and `key` must hold a single PEM-encoded certificate. The Job isn't started, and the Task keeps being retried, until the certificate can be read.
- The Tinkerbell service account needs `get` on the ConfigMap. The Helm chart doesn't grant it by default; add it with `rbac.additionalRoleRules`, limited to the ConfigMap by name:
  ```bash
  helm upgrade tinkerbell . \
    --reuse-values \
    --set-json 'rbac.additionalRoleRules=[{"apiGroups":[""],"resources":["configmaps"],"resourceNames":["provisioning-trust-anchor"],"verbs":["get"]}]'
  ```
- On Dell iDRAC, the BMC only accepts key store changes after `allowCustomKeys: true` has been applied, which happens on the next boot. Set it in an earlier Workflow, or in an earlier reboot cycle of the same Workflow, than the import.
- `certificateImport` adds to the database. To replace its contents instead, run `databaseResetKeys` with `DeleteAllKeys` in an earlier reboot cycle than the import.

#### Example: Enrolling a trust anchor for the installed OS

Hook OS is unsigned, so the PXE boot above runs with Secure Boot disabled. To have the installed OS boot with Secure Boot enforced, enroll its signing certificate once provisioning is done, in `postActions`. `allowCustomKeys` goes in `preparingActions` instead, so it is applied by the reboot into Hook OS, before the key operations are sent:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-secureboot-enroll
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "soft"
      - secureBoot:
          enable: false
      - secureBoot:
          allowCustomKeys: true
      - bootDevice:
          device: "pxe"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - powerAction: "soft"
      - secureBoot:
          databaseResetKeys:
            database: db
            resetType: ResetAllKeysToDefault
      - secureBoot:
          certificateImport:
            database: db
            certificatePEMConfigMapRef:
              name: provisioning-trust-anchor
              key: cert.pem
      - secureBoot:
          enable: true
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - powerAction: "on"
```

This configuration will:
1. Power off the machine, disable Secure Boot and allow custom keys, which the reboot into Hook OS applies
2. Set the next boot device to PXE and power on, running the Template with Secure Boot off
3. After provisioning, power off the machine
4. Restore the `db` database to its platform default, then add the certificate from the `cert.pem` key of the `provisioning-trust-anchor` ConfigMap to it
5. Re-enable Secure Boot, set the boot device back to disk persistently, and power on, which applies the staged changes
6. The installed OS boots with Secure Boot enforced, and its image is verified against the platform's default keys plus the added certificate

The same prerequisites as the previous example apply. Steps 4 and 5 rely on the BMC staging the operations and applying them together on the next boot, as Dell iDRAC does. If your BMC applies them in another order, or one at a time, put the key operations and `enable: true` in separate reboot cycles. Since `postActions` only run when the Workflow succeeds, a failed Workflow leaves the Machine with Secure Boot disabled and the old keys.

#### Secure Boot operations

The `secureBoot` action performs exactly one operation, set as one of its fields:

| Field | Operation |
|---|---|
| `enable` | Enable (`true`) or disable (`false`) UEFI Secure Boot. |
| `allowCustomKeys` | Enable or disable the platform's out-of-band acceptance of custom Secure Boot keys. Distinct from enabling Secure Boot itself: it only gates whether key store changes made through the BMC are accepted. Currently implemented for Dell iDRAC only. |
| `resetKeys` | Reset the key databases as a whole (see the `resetType` table above). |
| `databaseResetKeys` | Reset a single key database: `database` is `db`, `KEK` or `dbx`, and `resetType` is `ResetAllKeysToDefault` or `DeleteAllKeys`. To clear the PK use `resetKeys` with `DeletePK`. |
| `certificateImport` | Import a certificate into a single key database: `database` is `PK`, `KEK`, `db` or `dbx`. |

Support varies by board and BMC firmware: some BMCs have no key databases to manage, can't reset a single database, or can't import certificates. An operation the BMC can't perform fails its Task, and with it the Job and the Workflow, so only include the operations your boards support.

To run several operations, list one `secureBoot` action per operation. They are sent to the BMC in the order listed, one at a time. Many BMCs only stage these changes and apply them together on the next reboot, in an order the BMC decides, so put operations that depend on each other in separate reboot cycles (for example `preparingActions` and `postActions`) rather than relying on their order within one cycle.

### Templating in customboot

The `customboot` mode supports Go template syntax in action fields, enabling dynamic configuration based on Hardware specifications. This is particularly useful for virtual media URLs that need to include the Machine's MAC address.

#### Available Template Data

Templates have access to the full Hardware specification through the `.Hardware` variable:

- `{{ (index .Hardware.Interfaces 0).DHCP.MAC }}` - First interface MAC address in colon format (e.g., `52:54:00:12:34:01`)
- `{{ (index .Hardware.Interfaces 1).DHCP.MAC }}` - Second interface MAC address, etc.

#### Template Functions

Templates support [Sprig hermetic functions](https://masterminds.github.io/sprig/) for string manipulation and more:

- `replace` - String replacement (e.g., `replace ":" "-"` converts colons to dashes)
- `upper`, `lower` - Case conversion
- `trim`, `trimPrefix`, `trimSuffix` - String trimming
- And many more...

#### Example: Virtual Media with MAC-based URL

The most common use case is mounting a virtual CD-ROM with a URL that includes the Machine's MAC address:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example-iso-boot
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - powerAction: "off"
      - virtualMediaAction:
          # Template the MAC address in dash-separated format for ISO URL
          mediaURL: 'http://172.17.1.1:7080/iso/{{ (index .Hardware.Interfaces 0).DHCP.MAC | replace ":" "-" }}/hook.iso'
          kind: "CD"
      - bootDevice:
          device: "cdrom"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - powerAction: "off"
      - virtualMediaAction:
          mediaURL: ""  # Eject the ISO
          kind: "CD"
      - bootDevice:
          device: "disk"
          persistent: true
          efiBoot: true
      - powerAction: "on"
```

For a Hardware resource with MAC address `aa:bb:cc:dd:ee:ff`, the template would expand to:

```
http://172.17.1.1:7080/iso/aa-bb-cc-dd-ee-ff/hook.iso
```

#### Template Error Handling

If a template fails to parse or execute (e.g., accessing an interface that doesn't exist), the Workflow will transition to the `FAILED` state with an appropriate error message in the Workflow status conditions.
