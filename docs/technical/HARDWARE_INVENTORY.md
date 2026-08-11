# Hardware Inventory

This document explains Tinkerbell's hardware inventory: what it is, how to view the collected data, and how to use it in Workflow Templates. It then covers each collection path in detail.

## Overview

Tinkerbell can collect a machine's hardware inventory (CPUs, memory, drives, network interfaces, GPUs, firmware, and more) and record it on the Hardware object's status, where it can be viewed with `kubectl` and consumed by Workflow Templates.

Inventory is stored under `status.attributes`, organized by **collection path**:

| Collection path                        | Subtree                       | Source                                     | Availability                                      |
| -------------------------------------- | ----------------------------- | ------------------------------------------ | ------------------------------------------------- |
| [Out-of-band](#out-of-band-collection) | `status.attributes.outOfBand` | The machine's BMC (Baseboard Management Controller) | Available even when the machine is powered off |

## View the collected inventory

Inventory is stored on the Hardware object's status under `status.attributes`, keyed by collection path. For example, out-of-band data lives under `status.attributes.outOfBand`.

```bash
kubectl get hardware example-hardware -n tinkerbell -o jsonpath='{.status.attributes.outOfBand}' | jq
```

Example (abbreviated):

```yaml
status:
  attributes:
    outOfBand:
      lastUpdated: "2026-08-11T12:00:00Z"
      collectionMethod: gofish
      bios:
        vendor: Dell Inc.
        version: "2.19.1"
      product:
        manufacturer: Dell Inc.
        productName: PowerEdge R640
        serialNumber: ABC1234
      cpu:
        sockets:
          - slot: CPU1
            vendor: Intel
            model: Intel(R) Xeon(R) Gold 6248
            cores: 20
            threads: 40
      memory:
        totalBytes: 274877906944
        modules:
          - slot: DIMM.A1
            vendor: Micron
            sizeBytes: 34359738368
            speedMHz: 2933
      networkInterfaces:
        - ...
      blockDevices:
        - ...
```

Every field is optional. A missing field means the source did not report that detail, not that collection failed. What gets reported varies by collection path and, for out-of-band, by BMC vendor and protocol. The `collectionMethod` field records which source produced the data.

### Fields

All collection paths write the same source-agnostic attribute schema, so a subtree can include any of the following:

| Field                | Description                                             |
| -------------------- | ------------------------------------------------------- |
| `lastUpdated`        | Time this subtree was last refreshed.                   |
| `collectionMethod`   | Identifier of the source that produced the data.        |
| `cpu`                | Per-socket CPU detail (vendor, model, cores, threads).  |
| `memory`             | Total memory and per-module DIMM detail.                |
| `blockDevices`       | Storage drives.                                         |
| `networkInterfaces`  | Host network adapters (distinct from `spec.interfaces`).|
| `gpuDevices`         | GPU and accelerator devices.                            |
| `pciDevices`         | PCI devices. In-band collection only.                   |
| `chassis`            | Enclosure detail.                                       |
| `baseboard`          | Mainboard/motherboard detail.                           |
| `bios`               | System BIOS firmware.                                   |
| `product`            | Overall system identity (manufacturer, model, serial).  |
| `bmc`                | BMC firmware and management NIC.                        |
| `psus`               | Power supply units.                                     |
| `tpms`               | Trusted platform modules.                               |
| `storageControllers` | Storage controllers.                                    |

Some fields are only meaningful to a particular path. For example, the BMC's own firmware detail can only come from out-of-band collection, and `pciDevices` only from in-band. See the per-path sections below (e.g. [Out-of-band collection](#out-of-band-collection)) for details.

## Use the inventory in a Template

Workflow Templates can read the collected inventory to make provisioning decisions (for example selecting an install disk, branching on hardware attributes, or recording asset details into a provisioned OS) without hardcoding per-machine values.

The full Hardware object is exposed to Templates under the lowercase `hardware` key, addressed by its JSON field names. Inventory is therefore at `.hardware.status.attributes.<path>`, for example `.hardware.status.attributes.outOfBand`.

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Template
metadata:
  name: example-template
  namespace: tinkerbell
spec:
  data: |
    version: "0.1"
    name: example
    tasks:
      - name: provision
        worker: "{{.device_1}}"
        actions:
          - name: report-hardware
            image: alpine
            timeout: 60
            command: ["/bin/sh", "-c"]
            args:
              - |
                echo "BIOS vendor: {{ .hardware.status.attributes.outOfBand.bios.vendor }}"
                echo "Model: {{ .hardware.status.attributes.outOfBand.product.productName }}"
                echo "Collected via: {{ .hardware.status.attributes.outOfBand.collectionMethod }}"
```

> [!NOTE]
> Because inventory is refreshed asynchronously, a field may still be empty on a machine whose first collection has not completed. Guard against absent values in your Template rather than assuming a field is always present.

## Out-of-band collection

Out-of-band inventory is collected directly from a machine's BMC (Baseboard Management Controller). Because it is gathered over the network via the BMC rather than from a running operating system, it is available even when the machine is powered off or has never booted. It is written to `status.attributes.outOfBand`.

Collection is performed by Rufio's Machine controller using [bmclib](https://github.com/bmc-toolbox/bmclib), which talks to the BMC over Redfish or a vendor-specific API. The `collectionMethod` field records which bmclib driver produced the data: `gofish` for Redfish-based BMCs, or a vendor driver such as `dell`, `supermicro`, `asrockrack`, or `openbmc`.

> [!NOTE]
> IPMI-only BMCs cannot provide inventory. bmclib collects out-of-band inventory over Redfish or a vendor API only, so machines whose BMCs speak IPMI alone will not populate this subtree. This is an expected, permanent state for some fleets, not an error.

### Prerequisites

- The Rufio controller must be enabled (`deployment.envs.globals.enableRufioController=true`, the default).
- The Hardware object must reference a `machine.bmc.tinkerbell.org` object via `spec.bmcRef`.

  ```yaml
  spec:
    bmcRef:
      kind: machine.bmc.tinkerbell.org
      name: example-bmc
  ```

- The referenced BMC must be reachable and support Redfish or a bmclib vendor API. The `machine.bmc.tinkerbell.org` object should have a `status.conditions` entry of `type: Contactable` with `status: "True"`.

### Configuration

Out-of-band collection is enabled fleet-wide by default. It can be disabled fleet-wide, have its refresh interval adjusted, or be opted out for individual Hardware.

#### Enable or disable fleet-wide

- **CLI flag**: `--rufio-enable-inventory-collection=true|false`
- **Environment variable**: `TINKERBELL_RUFIO_ENABLE_INVENTORY_COLLECTION=true|false`
- **Helm value**: `deployment.envs.rufio.enableInventoryCollection`

```yaml
deployment:
  envs:
    rufio:
      enableInventoryCollection: true
```

#### Change the refresh interval

Accepts a Go duration string (e.g. `24h0m0s`, `12h`, `1h30m`).

- **CLI flag**: `--rufio-inventory-refresh-interval=24h0m0s`
- **Environment variable**: `TINKERBELL_RUFIO_INVENTORY_REFRESH_INTERVAL=24h0m0s`
- **Helm value**: `deployment.envs.rufio.inventoryRefreshInterval`

```yaml
deployment:
  envs:
    rufio:
      inventoryRefreshInterval: "24h0m0s"
```

#### Opt a single Hardware out

Set the following annotation on a Hardware object to exclude just that machine from out-of-band inventory collection (for example, for a BMC/firmware combination known to misbehave under Redfish inventory queries) without disabling the feature fleet-wide.

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: example-hardware
  annotations:
    tinkerbell.org/disable-outofband-inventory: "true"
```

#### Force an immediate refresh

To refresh a machine's inventory before its interval elapses, set the following annotation on the corresponding `machine.bmc.tinkerbell.org` object. It triggers an immediate refresh on the next reconcile and is removed automatically after a successful collection.

```bash
kubectl annotate machine.bmc.tinkerbell.org example-bmc \
  tinkerbell.org/refresh-inventory=true
```

### How it works

Collection is performed by Rufio's Machine controller, which already reconciles each Machine's power state on a fixed interval (the power check interval, default 30 minutes). During that reconcile it reuses the already-open BMC connection to collect inventory when a refresh is due. No second BMC connection is opened.

Inventory collection can be comparatively slow (5–30 seconds over Redfish), so it deliberately does not run on every power-poll reconcile. Instead, each machine is refreshed at most once per refresh interval (default 24 hours):

1. On each Machine reconcile, the controller finds the Hardware object whose `spec.bmcRef` points at the Machine.
1. If inventory has never been collected, is older than the refresh interval, or a manual refresh was requested, it collects inventory from the BMC.
1. The result is written to `status.attributes.outOfBand`, along with a `lastUpdated` timestamp and a `collectionMethod` recording which bmclib driver produced the data.

A small deterministic per-machine jitter (±10%) is added to the refresh interval so that machines onboarded or upgraded in bulk do not all become due in the same reconcile window.

Inventory collection is independent of Machine power/condition reconciliation. Failures (unreachable or unsupported BMCs) are logged and recorded as Kubernetes events, but never block the Machine reconcile.
