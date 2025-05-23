# Auto Enrollment in Tinkerbell

This document explains how Tinkerbell's auto enrollment feature works, how to enable it, how to configure a WorkflowRuleSet, and how to discover Agent attributes.

## Overview

Auto enrollment automatically creates a Workflow for Tink Agents without having the need for a pre-existing Hardware object. This is accomplished by matching Agent attributes against a set of rules defined in a WorkflowRuleSet. This allows for dynamic creation of Workflows based on Agent characteristics, such as its serial number, MAC address, and other hardware details.

## How Auto Enrollment works

When an Agent connects to the Tink Server:

1. The Agent sends its attributes (serial numbers, MAC addresses, etc.) to the Tink server.
1. If no workflow exists for the Agent, and auto enrollment is enabled, Tink server:
   1. Iterates through all WorkflowRuleSets and checks for a rule that matches the Agent's attributes.
   2. Creates a Workflow for the Agent based on the matched WorkflowRuleSet.
1. Tink Server serves the first Workflow Action to the Agent.
1. The Agent executes the Workflow Actions.

## How to enable Auto Enrollment

There is a CLI flag and an environment variable.

- **CLI flag**: `--tink-server-auto-enrollment-enabled=true`
- **Environment variable**: `TINKERBELL_TINK_SERVER_AUTO_ENROLLMENT_ENABLED=true`

In the Helm chart, use the following configuration in the `values.yaml` file:

```yaml
deployment:
  envs:
    tinkServer:
      autoEnrollmentEnabled: true
```

## How to configure a WorkflowRuleSet

WorkflowRuleSets are Kubernetes Custom Resource Definitions (CRDs). Here is an example WorkflowRuleSet.

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: WorkflowRuleSet
metadata:
  name: ruleset1
  namespace: tink-system
spec:
  rules:
  - '{"networkInterfaces": {"mac": [{"wildcard": "*"}]}}'
  workflow:
    addAttributes: true
    disabled: false
    namespace: tink-system
    template:
      agentValue: worker_id
      kvs:
        additional_value: im a value
      ref: sleep
```

### WorkflowRuleSet fields

- **rules [array]**: Rules is a list of Quamina patterns used to match against the attributes of an Agent. See [https://github.com/timbray/quamina/blob/main/PATTERNS.md](https://github.com/timbray/quamina/blob/main/PATTERNS.md]) for more information on the required format. All rules are combined using the `OR` operator. If any rule matches, the corresponding Workflow will be created.
- **workflow [object]**: Workflow holds the data used to configure the created Workflow.
  - **addAttributes [boolean]**: This indicates if the Agent attributes should be added as an Annotation in the created Workflow.
  - **disabled [boolean]**: Disabled indicates whether the Workflow will be enabled or not when created.
  - **namespace [string]**: The namespace to use when creating the Workflow.
  - **template [object]**: Data related to the configuration of the Template used in the created Workflow.
    - **agentValue [string]**: A value used in the referenced Template for the `Task[].worker` value. For example: "`device_id`" or "`worker_id`".
    - **kvs [map]**: Key-value pairs usable in the referenced Template.
    - **ref [string]**: The name of a Template object used in the created Workflow.

## How to discover Agent attributes

When starting out, it is recommended to create a WorkflowRuleSet that matches all Agents and disables running of a Workflow. This will create a disabled Workflow for each Agent that connects to Tink server. The Workflow will contain the Agent's attributes as an Annotation (`tinkerbell.org/agent-attributes`), which can be inspected to determine the Agent's characteristics for use in creating more specific rules. Attributes can be inspected using the following command:

```bash
kubectl get wf -n <namespace> enrollment-<agent id> -o jsonpath='{.metadata.annotations.tinkerbell\.org/agent-attributes}' | jq
```

Once familiar with the attributes, you can create more specific WorkflowRuleSets to match your environment.

> [!NOTE]  
> A disabled Workflow can be enabled using the following command:  
> `kubectl patch wf -n <namespace> enrollment-<agent id> -p '{"spec":{"disabled":false}}' --type merge`

### Example of Agent attributes

The following is an example of the attributes data structure and data types of an Agent. See also the Go struct definition [here](../../pkg/data/attributes.go).

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
        ]
      }
    ]
  },
  "memory": {
    "total": "32GB",
    "usable": "31GB"
  },
  "blockDevices": [
    {
      "name": "nvme0n1",
      "controllerType": "NVMe",
      "driveType": "SSD",
      "size": "239GB",
      "physicalBlockSize": "512B",
      "vendor": "unknown",
      "model": "KINGSTON ABCDEF-01"
    }
  ],
  "networkInterfaces": [
    {
      "name": "docker0",
      "mac": "",
      "speed": "10000Mb/s",
      "enabledCapabilities": [
        "tx-checksumming",
      ]
    },
    {
      "name": "eno1",
      "mac": "de:ad:be:ef:00:00",
      "speed": "1000Mb/s",
      "enabledCapabilities": [
        "auto-negotiation",
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
      "product": "11th Gen Core Processor PCIe Controller",
      "class": "Bridge",
      "driver": "pcieport"
    }
  ],
  "gpu": [
    {
      "vendor": "NVIDIA Corporation",
      "product": "GP107 [GeForce GTX 1050 Ti]",
      "class": "Display controller",
      "driver": "GP107"
    }
  ],
  "chassis": {
    "serial": "To Be Filled By O.E.M.",
    "vendor": "To Be Filled By O.E.M."
  },
  "bios": {
    "vendor": "American Megatrends International, LLC.",
    "version": "11.2233",
    "releaseDate": "12/13/2021"
  },
  "baseboard": {
    "vendor": "example vendor",
    "product": "ABC-DEF",
    "version": "",
    "serialNumber": "xxxxxxx"
  },
  "product": {
    "name": "abcd123",
    "vendor": "example vendor",
    "serialNumber": "xxxxxxx"
  }
}
```

## Workflow Creation

When a matching WorkflowRuleSet is found, a Workflow is created with the following:

1. The name is prefixed by `enrollment-`.
1. The owner reference is set to the matching WorkflowRuleSet.
1. If enabled adds Agent attributes as an annotation.

Given this example WorkflowRuleSet:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: WorkflowRuleSet
metadata:
  name: ruleset1
  namespace: tink-system
spec:
  rules:
  - '{"networkInterfaces": {"mac": [{"wildcard": "*"}]}}'
  workflow:
    addAttributes: true
    disabled: false
    namespace: tink-system
    template:
      agentValue: worker_id
      kvs:
        additional_value: im a value
      ref: example
```

The following Workflow will be created for any Agent:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Workflow
metadata:
  name: enrollment-hello123456
  namespace: tink-system
  ownerReferences:
  - apiVersion: tinkerbell.org/v1alpha1
    kind: WorkflowRuleSet
    name: ruleset1
    uid: a8a6e8a7-6a8c-4fee-bcba-2318e4b7ae5b
  resourceVersion: "374571"
  uid: f81d3e11-d9c7-46bd-aa0b-fbfa16c90ca4
spec:
  disabled: false
  hardwareMap:
    additional_value: im a value
    worker_id: hello123456
  templateRef: example
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
kubectl logs -l app=tinkerbell
```
