# Workflow Boot Modes

This document describes the different boot modes available in the Workflow `spec.bootOptions.bootMode`.

## netboot

```yaml
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

[Reference](../../tink/controller/internal/workflow/pre.go#L49-L72)

## isoboot

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
        mediaURL: "http://tinkerbell-ip:7171/iso/:macAddress/hook.iso"
        kind: "CD"
    - oneTimeBootDeviceAction:
        device:
          - "cdrom"
        efiBoot: false # or true based on Hardware.spec.interfaces[].dhcp.uefi
    - powerAction: "on"
```

[Reference](../../tink/controller/internal/workflow/pre.go#L106-L141)

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

[Reference](../../tink/controller/internal/workflow/post.go#L35-L42)

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

[PREPARING Reference](../../tink/controller/internal/workflow/pre.go#L158)  
[POST Reference](../../tink/controller/internal/workflow/post.go#L61)
