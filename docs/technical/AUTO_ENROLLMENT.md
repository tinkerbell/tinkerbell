# Auto Enrollment in Tinkerbell

This document explains how Tinkerbell's auto enrollment feature works, how to enable it, how to configure a WorkflowRuleSet, and how to discover Agent attributes.

## Overview

Auto enrollment automatically creates a Workflow for Tink Agents without having the need for a pre-existing Hardware object. This is accomplished by matching Agent attributes against a set of rules defined in a WorkflowRuleSet. This allows for dynamic creation of Workflows based on Agent characteristics, such as its serial number, MAC address, and other hardware details.

## How Auto Enrollment works

When an Agent connects to the Tink Server:

1. The Agent sends its attributes (serial numbers, MAC addresses, etc.) to the Tink server
1. If no workflow exists for the Agent, and auto enrollment is enabled, Tink server:
   - Searches for WorkflowRuleSets that match the Agent's attributes
   - Creates a Workflow for the Agent based on the matched rule set
1. The Agent then executes the Workflow

## How to enable Auto Enrollment

Theres a CLI flag and an environment variable.

## How to configure a WorkflowRuleSet

WorkflowRuleSets are Kubernetes Custom Resource Definitions (CRDs).



## How to discover Agent attributes

When the WorkflowRuleSet has `spec.addAttributesToStatus: true` then matching Agents that send attributes will have those attributes added to the Workflow `status.agentAttributes` field. These attributes can be inspected in order to create WorkflowRuleSet rules. When used in conjunction with a WorkflowRuleSet `spec.workflow.disabled: true`, the attributes can be inspected before a Workflow is executed.
Example JSON :

```json
{
  "cpu": {
    "totalCores": 4,
    "totalThreads": 8,
    "processors": [
      {
        "id": 0,
        "cores": 4,
        "threads": 8,
        "vendor": "GenuineIntel",
        "model": "11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz",
        "capabilities": [
          "fpu",
          "vme",
          "de",
          "pse",
          "tsc",
          "msr",
          "pae",
          "mce",
          "cx8",
          "apic",
          "sep",
          "mtrr",
          "pge",
          "mca",
          "cmov",
          "pat",
          "pse36",
          "clflush",
          "dts",
          "acpi",
          "mmx",
          "fxsr",
          "sse",
          "sse2",
          "ss",
          "ht",
          "tm",
          "pbe",
          "syscall",
          "nx",
          "pdpe1gb",
          "rdtscp",
          "lm",
          "constant_tsc",
          "art",
          "arch_perfmon",
          "pebs",
          "bts",
          "rep_good",
          "nopl",
          "xtopology",
          "nonstop_tsc",
          "cpuid",
          "aperfmperf",
          "tsc_known_freq",
          "pni",
          "pclmulqdq",
          "dtes64",
          "monitor",
          "ds_cpl",
          "vmx",
          "smx",
          "est",
          "tm2",
          "ssse3",
          "sdbg",
          "fma",
          "cx16",
          "xtpr",
          "pdcm",
          "pcid",
          "sse4_1",
          "sse4_2",
          "x2apic",
          "movbe",
          "popcnt",
          "tsc_deadline_timer",
          "aes",
          "xsave",
          "avx",
          "f16c",
          "rdrand",
          "lahf_lm",
          "abm",
          "3dnowprefetch",
          "cpuid_fault",
          "epb",
          "cat_l2",
          "invpcid_single",
          "cdp_l2",
          "ssbd",
          "ibrs",
          "ibpb",
          "stibp",
          "ibrs_enhanced",
          "tpr_shadow",
          "vnmi",
          "flexpriority",
          "ept",
          "vpid",
          "ept_ad",
          "fsgsbase",
          "tsc_adjust",
          "bmi1",
          "avx2",
          "smep",
          "bmi2",
          "erms",
          "invpcid",
          "rdt_a",
          "avx512f",
          "avx512dq",
          "rdseed",
          "adx",
          "smap",
          "avx512ifma",
          "clflushopt",
          "clwb",
          "intel_pt",
          "avx512cd",
          "sha_ni",
          "avx512bw",
          "avx512vl",
          "xsaveopt",
          "xsavec",
          "xgetbv1",
          "xsaves",
          "split_lock_detect",
          "dtherm",
          "ida",
          "arat",
          "pln",
          "pts",
          "avx512vbmi",
          "umip",
          "pku",
          "ospke",
          "avx512_vbmi2",
          "gfni",
          "vaes",
          "vpclmulqdq",
          "avx512_vnni",
          "avx512_bitalg",
          "tme",
          "avx512_vpopcntdq",
          "rdpid",
          "movdiri",
          "movdir64b",
          "fsrm",
          "avx512_vp2intersect",
          "md_clear",
          "flush_l1d",
          "arch_capabilities"
        ]
      }
    ]
  },
  "memory": { "total": "32GB", "usable": "31GB" },
  "blockDevices": [
    {
      "name": "nvme0n1",
      "controllerType": "NVMe",
      "driveType": "SSD",
      "size": "239GB",
      "physicalBlockSize": "512B",
      "vendor": "unknown",
      "model": "KINGSTON OM8PDP3256B-A01"
    }
  ],
  "networkInterfaces": [
    {
      "name": "br-3d1549d4f99f",
      "mac": "",
      "speed": "Unknown!",
      "enabledCapabilities": [
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "netns-local",
        "tx-gso-robust",
        "tx-fcoe-segmentation",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-gso-partial",
        "tx-tunnel-remcsum-segmentation",
        "tx-sctp-segmentation",
        "tx-esp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert"
      ]
    },
    {
      "name": "cni0",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "netns-local",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-gso-partial",
        "tx-tunnel-remcsum-segmentation",
        "tx-sctp-segmentation",
        "tx-esp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert"
      ]
    },
    {
      "name": "docker0",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "netns-local",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-gso-partial",
        "tx-tunnel-remcsum-segmentation",
        "tx-sctp-segmentation",
        "tx-esp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert"
      ]
    },
    {
      "name": "eno1",
      "mac": "a8:a1:59:d0:e2:52",
      "speed": "1000Mb/s",
      "enabledCapabilities": [
        "auto-negotiation",
        "rx-checksumming",
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "rx-vlan-offload",
        "tx-vlan-offload",
        "receive-hashing",
        "highdma"
      ]
    },
    {
      "name": "flannel.1",
      "mac": "",
      "speed": "1000Mb/s",
      "enabledCapabilities": [
        "auto-negotiation",
        "rx-checksumming",
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-lockless",
        "tx-sctp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list"
      ]
    },
    {
      "name": "tailscale0",
      "mac": "",
      "speed": "Unknown!",
      "enabledCapabilities": [
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-vlan-offload",
        "tx-lockless",
        "tx-vlan-stag-hw-insert"
      ]
    },
    {
      "name": "veth3be0d58",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "rx-checksumming",
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "tx-checksum-sctp",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "rx-vlan-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-sctp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert",
        "rx-vlan-stag-hw-parse"
      ]
    },
    {
      "name": "veth867f6914",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "rx-checksumming",
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "tx-checksum-sctp",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "rx-vlan-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-sctp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert",
        "rx-vlan-stag-hw-parse"
      ]
    },
    {
      "name": "vethda0f174",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "rx-checksumming",
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "tx-checksum-sctp",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "rx-vlan-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-sctp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert",
        "rx-vlan-stag-hw-parse"
      ]
    },
    {
      "name": "virbr0",
      "mac": "",
      "speed": "Unknown!",
      "enabledCapabilities": [
        "tx-checksumming",
        "tx-checksum-ip-generic",
        "scatter-gather",
        "tx-scatter-gather",
        "tx-scatter-gather-fraglist",
        "tcp-segmentation-offload",
        "tx-tcp-segmentation",
        "tx-tcp-ecn-segmentation",
        "tx-tcp-mangleid-segmentation",
        "tx-tcp6-segmentation",
        "generic-segmentation-offload",
        "generic-receive-offload",
        "tx-vlan-offload",
        "highdma",
        "tx-lockless",
        "netns-local",
        "tx-gso-robust",
        "tx-fcoe-segmentation",
        "tx-gre-segmentation",
        "tx-gre-csum-segmentation",
        "tx-ipxip4-segmentation",
        "tx-ipxip6-segmentation",
        "tx-udp_tnl-segmentation",
        "tx-udp_tnl-csum-segmentation",
        "tx-gso-partial",
        "tx-tunnel-remcsum-segmentation",
        "tx-sctp-segmentation",
        "tx-esp-segmentation",
        "tx-udp-segmentation",
        "tx-gso-list",
        "tx-vlan-stag-hw-insert"
      ]
    }
  ],
  "pciDevices": [
    {
      "vendor": "Intel Corporation",
      "product": "11th Gen Core Processor Host Bridge/DRAM Registers",
      "class": "Bridge"
    },
    {
      "vendor": "Intel Corporation",
      "product": "TigerLake-LP GT2 [Iris Xe Graphics]",
      "class": "Display controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "TigerLake-LP Dynamic Tuning Processor Participant",
      "class": "Signal processing controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "11th Gen Core Processor PCIe Controller",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Thunderbolt 4 PCI Express Root Port #2",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Thunderbolt 4 PCI Express Root Port #3",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tigerlake Telemetry Aggregator Driver",
      "class": "Signal processing controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Thunderbolt 4 USB Controller",
      "class": "Serial bus controller",
      "driver": "xhci_hcd"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Thunderbolt 4 NHI #1",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP USB 3.2 Gen 2x1 xHCI Host Controller",
      "class": "Serial bus controller",
      "driver": "xhci_hcd"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Shared SRAM",
      "class": "Memory controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO I2C Controller #0",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO I2C Controller #2",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO I2C Controller #3",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Management Engine Interface",
      "class": "Communication controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Active Management Technology - SOL",
      "class": "Communication controller",
      "driver": "serial"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO I2C Controller #4",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO I2C Controller #5",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "unknown",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tigerlake PCH-LP PCI Express Root Port #6",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP PCI Express Root Port #8",
      "class": "Bridge",
      "driver": "pcieport"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO UART Controller #0",
      "class": "Communication controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Serial IO SPI Controller #1",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP LPC Controller",
      "class": "Bridge"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP Smart Sound Technology Audio Controller",
      "class": "Multimedia controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP SMBus Controller",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Tiger Lake-LP SPI Controller",
      "class": "Serial bus controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Ethernet Connection (13) I219-LM",
      "class": "Network controller",
      "driver": "e1000e"
    },
    {
      "vendor": "Kingston Technology Company, Inc.",
      "product": "OM3PDP3 NVMe SSD",
      "class": "Mass storage controller",
      "driver": "nvme"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Wi-Fi 6 AX200",
      "class": "Network controller"
    },
    {
      "vendor": "Intel Corporation",
      "product": "Ethernet Controller I225-LM",
      "class": "Network controller"
    }
  ],
  "chassis": {
    "serial": "To Be Filled By O.E.M.",
    "vendor": "To Be Filled By O.E.M."
  },
  "bios": {
    "vendor": "American Megatrends International, LLC.",
    "version": "P1.50J",
    "releaseDate": "12/13/2021"
  },
  "baseboard": {
    "vendor": "ASRock",
    "product": "NUC-TGL",
    "version": "",
    "serialNumber": "T80-F5002000130"
  },
  "product": {
    "name": "LLN11CRv5",
    "vendor": "Simply NUC",
    "serialNumber": "7B60007P"
  }
}
```


## Configuration

To enable auto enrollment in your Tinkerbell deployment:

```yaml
# In your Tinkerbell configuration (e.g., helm values)
deployment:
  envs:
    globals:
      enableTinkController: true
    tinkServer:
      autoEnrollment: true
```

## Workflow Rule Sets

A WorkflowRuleSet defines the rules for matching agents to workflows. A WorkflowRuleSet contains:

- **rules**: JSON patterns that match against agent attributes
- **workflowNamespace**: The namespace where the workflow will be created
- **workflow**: Configuration for the created workflow, including template reference
- **addAttributesAsLabels**: Whether to add agent attributes as labels on the workflow

### Matching Rules

Rules are defined as JSON patterns that match against the agent's attributes. The matching uses [quamina](https://github.com/timbray/quamina), a pattern-matching engine.

Example patterns:

```json
{"chassis": {"serial": ["12345"]}}
{"network": {"interfaces": [{"mac": ["00:00:00:00:00:01"]}]}}
```

### Example WorkflowRuleSet

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: WorkflowRuleSet
metadata:
  name: dell-r6515-ruleset
spec:
  rules:
  - '{"chassis": {"manufacturer": ["Dell Inc."], "product": ["PowerEdge R6515"]}}'
  workflowNamespace: default
  agentTemplateValue: worker
  addAttributesAsLabels: true
  workflow:
    templateRef:
      name: ubuntu-template
      namespace: default
    templateKVPairs:
      os: ubuntu
      version: "22.04"
```

## Attribute Matching

The auto enrollment system attempts to find the best match for an agent by:

1. Evaluating all WorkflowRuleSets against the agent's attributes
2. Selecting the rule set with the most matching patterns
3. Using that rule set to create a workflow

The agent attributes include:

- Chassis information (serial, manufacturer, product)
- BMC details
- Network interfaces
- Storage devices
- CPU information
- Memory configuration

## Workflow Creation

When a matching rule set is found, Tinkerbell:

1. Creates a workflow with a name prefixed by `enrollment-`
2. Sets the owner reference to the matching WorkflowRuleSet
3. Applies the template reference from the rule set
4. Maps the agent ID to the specified hardware map entry
5. Adds agent attributes as labels if configured
6. Creates the workflow in the specified namespace

Example generated workflow:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Workflow
metadata:
  name: enrollment-worker-123abc
  namespace: default
  labels:
    chassis.serial: "12345"
    chassis.manufacturer: "Dell Inc."
  ownerReferences:
  - apiVersion: tinkerbell.org/v1alpha1
    kind: WorkflowRuleSet
    name: dell-r6515-ruleset
    uid: 12345678-1234-1234-1234-123456789012
spec:
  templateRef:
    name: ubuntu-template
    namespace: default
  hardwareMap:
    worker: worker-123abc
    os: ubuntu
    version: "22.04"
```

## Troubleshooting

Common issues:

1. **No matching WorkflowRuleSet found**
   - Verify the agent attributes match at least one rule
   - Check rules syntax for errors
   - Enable debug logging on the server

2. **Workflow creation fails**
   - Check permissions for creating workflows in the target namespace
   - Verify the template referenced in the rule set exists

3. **Workflow not running**
   - Check that the workflow controller is running
   - Verify the workflow is not disabled
   - Check the template is valid

Useful commands:

```bash
# List WorkflowRuleSets
kubectl get workflowrulesets

# Describe a WorkflowRuleSet
kubectl describe workflowruleset <name>

# Check server logs
kubectl logs -l app=tinkerbell -c server
```

## Reference

This implementation leverages several key components:

- **WorkflowRuleSet**: Custom Resource Definition that defines matching rules
- **Enrollment Handler**: Server-side logic that processes matching and workflow creation
- **Retry Mechanism**: Ensures reliable workflow creation with exponential backoff
- **Agent Attributes**: Properties sent by the agent that are used for rule matching

For more details on the implementation, see:

- [`tink/server/internal/grpc/enrollment.go`](tink/server/internal/grpc/enrollment.go)
- [Tinkerbell API Documentation](https://docs.tinkerbell.org/services/tink-server/)
- [Quamina Documentation](https://github.com/timbray/quamina)
