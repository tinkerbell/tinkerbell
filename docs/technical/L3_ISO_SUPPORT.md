# Layer 3 Provisioning (ISO Boot) in Tinkerbell

This document describes what layer 3 provisioning (ISO boot) in Tinkerbell is, how it works, and how to use it.

## Prerequisites

In order to use layer 3 provisioning (ISO boot) in Tinkerbell, you need the following prerequisites:

1. A compatible BMC with Redfish support that implements virtual media mounting
1. Layer 3 network access between Tinkerbell and the target BMC

## What is Layer 3 Provisioning (ISO Boot)?

Layer 3 provisioning in Tinkerbell is a method of provisioning a machine by running the Tinkerbell operating system installation environment, HookOS, from an ISO file that is booted from a virtual CDROM/DVD device. This is accomplished in Tinkerbell by utilizing the virtual media mounting enabled [Redfish](https://redfish.dmtf.org/) endpoint of a [BMC](https://en.wikipedia.org/wiki/Baseboard_management_controller).

## How Does Layer 3 Provisioning (ISO Boot) in Tinkerbell Work?

### Step 1

Tinkerbell tells the BMC the HTTP or HTTPS location of the ISO file to be mounted as virtual media.
The format of the URL is as follows: `http(s)://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MAC_ADDRESS>/hook.iso`

- The `TINKERBELL_IP_OR_HOSTNAME` is defined by either `--public-ipv4` or `--bind-addr` or `--ipxe-http-script-bind-addr` or a hostname that resolves to the Tinkerbell server.
- The `PORT` is defined with either `--ipxe-http-script-bind-port` for HTTP or `--https-bind-port` for HTTPS.
- The `MAC_ADDRESS` is the MAC address of one of the target machine's network interfaces. This is needed so that Tinkerbell can add kernel command line parameters specific to that machine.

### Step 2

The BMC mounts the ISO file and makes it available to the target machine as a virtual CDROM/DVD device. This mounting typically happens using fuse over HTTP(S), also known as HTTPFS.

> [!NOTE]
> HTTPFS uses range requests to fetch only the parts of the ISO file that are needed at any given time, rather than downloading the entire ISO file at once.
> This means that the BMC will do many HTTP 206 Partial Content requests to the Tinkerbell server as the target machine boots from the ISO.

### Step 3

When the BMC requests the ISO file from Tinkerbell, Tinkerbell acts as a reverse proxy serving and patching a source ISO.
The source ISO is defined by one of the following locations, in order of precedence:

1. URL query parameter on the ISO request. For example:

   ```bash
   http(s)://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MAC_ADDRESS>/hook.iso?sourceISO=<url>
   ```

> [!NOTE]
> Not all BMC vendors support query parameters. Validate that your BMC vendor supports query parameters before using this method.

2. The `spec.interfaces[].isoboot.sourceISO` field in the Hardware object corresponding to the MAC Address in the URL.
3. The source ISO defined by the CLI flag `--iso-upstream-url`.

## How to Use Layer 3 Provisioning (ISO Boot) in Tinkerbell

There are 3 options for using layer 3 provisioning (ISO boot) in Tinkerbell:

1. Configure a Workflow's `spec.bootOptions` to use ISO boot.
1. Create a `job.bmc.tinkerbell.org` object.
1. Manually mount the ISO file using the BMC's web interface or Redfish API.

### Configure a Workflow's `spec.bootOptions` to Use ISO Boot

To configure a Workflow to use ISO boot, set the following `bootOptions` in the Workflow:

```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Workflow
metadata:
  name: example
spec:
  bootOptions:
    bootMode: "isoboot"
    isoURL: "https://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MACAddress>/hook.iso"
```

### Create a `job.bmc.tinkerbell.org` Object

The following `job.bmc.tinkerbell.org` object demonstrates how to use ISO boot in a Job. See the [Rufio documentation](/docs/technical/rufio/README.md) for more details on creating Jobs with Rufio.

```yaml
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Job
metadata:
  name: example
spec:
  machineRef:
    name: example
    namespace: example
  tasks:
    - powerAction: "off"
    - virtualMediaAction:
        mediaURL: ""
        kind: "CD"
    - virtualMediaAction:
        mediaURL: "https://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MACAddress>/hook.iso"
        kind: "CD"
    - oneTimeBootDeviceAction:
        device:
          - "cdrom"
        efiBoot: true
    - powerAction: "on"
```

There are 2 `virtualMediaAction` tasks in this example Job. They both are of `kind: "CD"`. The first `virtualMediaAction` has an empty `mediaURL`, which tells the BMC to unmount any previously mounted virtual media. The second `virtualMediaAction` mounts the ISO file from Tinkerbell.

### Manually Mount the ISO File Using the BMC's Web Interface or Redfish API

You can also manually mount the ISO file using your BMC's web interface or Redfish API. The exact steps will vary depending on your BMC vendor and model, so refer to your vendor's BMC documentation for instructions on how to mount virtual media.
