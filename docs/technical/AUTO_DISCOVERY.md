# Auto Discovery in Tinkerbell

This document explains how Tinkerbell's auto discovery feature works and how to enable it.

## Overview

Auto discovery allows Tinkerbell to automatically create Hardware objects for machines that do not have one. This feature useful when onboarding new hardware devices that are not yet registered in the Tinkerbell system.

## How Auto Discovery Works

When an Agent connects to Tink Server:

1. The Agent sends its attributes (serial numbers, MAC addresses, etc.) to the Tink server.
1. If no workflow exists for the Agent, and auto discovery is enabled, Tink server:
   1. Checks if an existing Hardware object exists for the Agent in the configured namespace. The name of the Hardware object is `discovery-{Agent ID}`.
   1. If no Hardware object exists, Tink server creates a new Hardware object with the name `discovery-{Agent ID}` and populates it with the Agent's attributes.

## How to Enable Auto Discovery

There is a CLI flag and an environment variable to enable auto discovery.

- **CLI flag**: `--tink-server-auto-discovery-enabled=true`
- **Environment variable**: `TINKERBELL_TINK_SERVER_AUTO_DISCOVERY_ENABLED=true`

In the Helm chart, use the following configuration in the `values.yaml` file:

```yaml
deployment:
  envs:
    tinkServer:
      autoDiscoveryEnabled: true
```

or set the Helm value from the CLI:

```bash
--set "deployment.envs.tinkServer.autoDiscoveryEnabled=true"
```

## Configuring Auto Discovery

Auto discovery has one configuration option. This option is for the namespace where new Hardware objects will be created. The default namespace is `default`.
There is a CLI flag and an environment variable to set the namespace.

- **CLI flag**: `--tink-server-auto-discovery-namespace=<namespace>`
- **Environment variable**: `TINKERBELL_TINK_SERVER_AUTO_DISCOVERY_NAMESPACE=<namespace>`

In the Helm chart, use the following configuration in the `values.yaml` file:

```yaml
deployment:
  envs:
    tinkServer:
      autoDiscoveryNamespace: <namespace>
```

or set the Helm value from the CLI:

```bash
--set "deployment.envs.tinkServer.autoDiscoveryNamespace=<namespace>"
```

## Hardware Object Creation

When a Hardware object is created by auto discovery, the following fields are populated. The example `Value` below are only examples and will vary based on the Agent's actual attributes and configuration.

| Field | Value | description |
|-------|-------|-------------|
| `metadata.name` | `discovery-{Agent ID}` | Unique name for the discovered hardware. |
| `metadata.namespace` | `tinkerbell` | Namespace where the Hardware object is created, configured in the Tink server. |
| `metadata.labels` | `{"tinkerbell.org/auto-discovered": "true"}` | Label indicating that this Hardware object was created by auto discovery. |
| `metadata.annotations` | `tinkerbell.org/agent-attributes: '{"cpu":...}'` | Contains the full Agent attributes in JSON format. |
| `spec.disks` | `- device: /dev/sda` | All disks, from the Agent attributes, with a non empty size will be added to the `spec.disks` list. |
| `spec.interfaces.dhcp` | `- mac: {MAC address}` | All non empty MAC address, from the Agent attributes, will be added to a new item in the `spec.interfaces` list. |

## Troubleshooting

If you encounter issues with auto discovery, check the following:

- Ensure that the Tink server is running with the auto discovery feature enabled.
- Verify that the Agent is sending its attributes correctly.
  - Check the Tinkerbell and Agent logs for the attributes.
- Ensure that the namespace for auto discovery is correctly configured and exists in your Kubernetes cluster.
- Enable more logging in the Tink server to get more details about the auto discovery process. You can do this by setting the `TINKERBELL_TINK_SERVER_LOG_LEVEL` environment variable to > 0.
