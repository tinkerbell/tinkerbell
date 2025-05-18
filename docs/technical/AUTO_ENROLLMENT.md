# Auto Enrollment in Tinkerbell

This document explains how Tinkerbell's auto enrollment feature works, how to enable it, how to configure a WorkflowRuleSet, and how to discover Agent attributes.

## Overview

Auto enrollment automatically assigns Workflows to Tink Agents without having the need for a pre-existing Hardware object by matching the agent's attributes against a set of rules defined in a WorkflowRuleSet. This allows for dynamic provisioning of workflows based on the agent's characteristics, such as its serial number, MAC address, and other hardware details.

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
