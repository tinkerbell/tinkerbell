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

[PREPARING Reference](/tink/controller/internal/workflow/pre.go#L157)
[POST Reference](/tink/controller/internal/workflow/post.go#L57)

### Webhooks in customboot

`preparingActions` and `postActions` can also call arbitrary HTTP endpoints — e.g. to notify an
external system, or gate on something outside Tinkerbell — interleaved with BMC actions in the
same ordered sequence. Each element is either a BMC action, written exactly as shown above
(`- powerAction: "off"`, `- bootDevice: {...}`, no wrapper key needed), or a `webhook:` entry.
Webhook calls are made directly by the Tink Controller (no BMC or `job.bmc.tinkerbell.org`
involved); contiguous BMC-action steps are still batched into a single `job.bmc.tinkerbell.org`
— only a webhook in between forces a new one:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example
spec:
  templateRef: example
  hardwareRef: example
  bootOptions:
    bootMode: customboot
    custombootConfig:
      preparingActions:
      - webhook:
          url: "https://inventory.example.com/hosts/{{ (index .Hardware.Interfaces 0).DHCP.MAC | replace \":\" \"-\" }}/power-off-requested"
      - powerAction: "off"
      - webhook:
          url: "https://inventory.example.com/hosts/{{ (index .Hardware.Interfaces 0).DHCP.MAC | replace \":\" \"-\" }}/power-off-confirmed"
      - bootDevice:
          device: "pxe"
          efiBoot: true
      - powerAction: "on"
      postActions:
      - powerAction: "on"
      - webhook:
          url: "https://inventory.example.com/hosts/{{ (index .Hardware.Interfaces 0).DHCP.MAC | replace \":\" \"-\" }}/provisioning-complete"
```

A pure-BMC-actions list (no `webhook:` entries at all) behaves identically to how
`preparingActions`/`postActions` always have — there's no cost to writing a plain BMC-only list
this way, whether or not you use webhooks.

A webhook call fails the Workflow the same way a failed BMC action does — there's no automatic
retry. Progress through the list is tracked in `status.bootOptions.actions`, so an
already-succeeded step is never re-run on a later reconcile. (One exception: a Workflow that
completed its `postActions` *before* upgrading to a Tinkerbell version with this feature has no
such tracking recorded yet, so its `postActions` run once more on the first reconcile after the
upgrade.)

#### Webhook headers and authentication

`headers` is a list of `{name, value}` for static (still templated) values, or
`{name, valueFrom: {name, key}}` to source a header directly from a Secret — useful for a Bearer
token, which is never templated so it can't leak into logs via a failed render:

```yaml
      preparingActions:
      - webhook:
          url: "https://inventory.example.com/notify"
          headers:
          - name: X-Correlation-ID
            value: "{{ .Hardware.Metadata.State }}"
          - name: Authorization
            valueFrom:
              name: inventory-webhook-token
              key: bearer-token
```

For HTTP Basic auth, `basicAuth` references a Secret containing `username` and `password` keys —
the request's `Authorization` header is built and base64-encoded automatically, no pre-encoding
required:

```yaml
      preparingActions:
      - webhook:
          url: "https://inventory.example.com/notify"
          basicAuth:
            name: inventory-webhook-creds
            namespace: tinkerbell
```

`basicAuth` takes precedence over a `headers` entry named `Authorization` if both are set on the
same webhook — don't combine them on the same call.

[`WebhookAction` reference](/tink/controller/internal/workflow/webhook.go)
[`preparingActions`/`postActions` execution reference](/tink/controller/internal/workflow/customboot.go)

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

### Templating in customboot

The `customboot` mode supports Go template syntax in action fields, enabling dynamic configuration based on Hardware specifications. This is particularly useful for virtual media URLs that need to include the Machine's MAC address. The same templating applies to a `webhook`'s `url`, `body`, and static header `value` fields — a `webhook.valueFrom`/`basicAuth`-sourced value is never templated (see [Webhooks in customboot](#webhooks-in-customboot)).

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
