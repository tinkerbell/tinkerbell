# Layer 3 Provisioning (ISO Boot) in Tinkerbell
This document describes what layer 3 provisioning (ISO boot) in Tinkerbell is, how it works, and how to use it.
## Prerequisites
In order to use layer 3 provisioning (ISO boot) in Tinkerbell, you need the following prerequisites:
1. A compatible BMC with Redfish support that implements virtual media mounting
1. Layer 3 network access between Tinkerbell and the target BMC and machine
If using 
1. A `hardare.tinkerbell.org` object defined for the target machine with at least one network interface and `spec.bmcRef` defined
1. A `machine.bmc.tinkerbell.org` object defined for the target machine's BMC
## What is Layer 3 Provisioning (ISO Boot)?
Layer 3 provisioning in Tinkerbell is a method of provisioning a machine by running the Tinkerbell operating system installation environment, HookOS, from an ISO file that is booted from a virtual CDROM/DVD device. This is accomplished in Tinkerbell by utilizing the virtual media mounting enabled [Redfish](https://redfish.dmtf.org/) endpoint of a [BMC](https://en.wikipedia.org/wiki/Baseboard_management_controller).
## How Does It Work?
Step 1:  
Tinkerbell tells the BMC the HTTP or HTTPS location of the ISO file to be mounted as virtual media.
The format of the URL is as follows:
http(s)://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MAC_ADDRESS>/hook.iso
- The `TINKERBELL_IP_OR_HOSTNAME` is defined by either `--public-ipv4` or `--bind-addr` or `--ipxe-http-script-bind-addr` or a hostname that resolves to the Tinkerbell server.
- The `PORT` is defined with either `--ipxe-http-script-bind-port` for HTTP or `--https-bind-port` for HTTPS.
- The `MAC_ADDRESS` is the MAC address of one of the target machine's network interfaces. This is needed so that Tinkerbell can add kernel command line parameters specific to that machine.
Step 2:  
The BMC mounts the ISO file and makes it available to the target machine as a virtual CDROM/DVD device. This mounting typically happens using fuse over HTTP(S), also known as HTTPFS.
> [!NOTE] HTTPFS uses range requests to fetch only the parts of the ISO file that are needed at any given time, rather than downloading the entire ISO file at once.
> This means that the BMC will do many HTTP 206 Partial Content requests to the Tinkerbell server as the target machine boots from the ISO.
Step 3:  
When the BMC requests the ISO file from Tinkerbell, Tinkerbell acts as a reverse proxy serving and patching a source ISO.
The source ISO is defined by one of the following locations, in order of precedence:
1. URL query parameter on the ISO request URL. For example:
   ```
   http(s)://<TINKERBELL_IP_OR_HOSTNAME>:<PORT>/iso/<MAC_ADDRESS>/hook.iso?sourceISO=<url>
   ```
2. The `spec.interface[].isoboot.sourceISO` field in the Hardware object corresponding to the MAC Address in the URL.
3. The source ISO defined by the CLI flag `--iso-upstream-url`.
## Overview
Tinkerbell supports dynamic source ISO selection and multi-architecture ISO booting through three mechanisms:
1. Per-hardware ISO configuration in Hardware objects
2. Runtime override via URL query parameters
3. Default fallback configuration
This flexibility allows operators to:
- Deploy different operating systems to different hardware architectures
- Override ISO sources dynamically without changing configurations
- Maintain default images while supporting exceptions
- Simplify management of heterogeneous hardware fleets
## Architecture
### Hardware Object Extension
The Hardware API now includes an `isoboot` configuration in the interfaces specification:
```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: arm64-server
spec:
  interfaces:
    - isoboot:
        sourceISO: "https://example.com/hook-arm64.iso"
The `sourceISO` field must be:
- A valid URL with HTTP or HTTPS scheme
- Accessible by the Tinkerbell server
- Point to a valid bootable ISO image
### Source Selection Logic
ISO selection follows a strict precedence order:
1. **Query Parameter** (`?sourceISO=<url>`):
   - Highest priority
   - Enables runtime overrides
   - Must be a valid HTTP(S) URL
   - Example: `GET /iso/01:23:45:67:89:ab/hook.iso?sourceISO=https://example.com/custom.iso`
2. **Hardware Object** (`spec.interfaces[].isoboot.sourceISO`):
   - Applied when no query parameter is present
   - Persists across boots
   - Configured per hardware device
3. **Default Configuration**:
   - Lowest priority
   - Used when no other sources are specified
   - Set via Tinkerbell configuration
### URL Validation
All ISO URLs undergo strict validation:
- Must use `http://` or `https://` schemes
- Must be properly formatted URLs
- Invalid URLs result in boot failures with appropriate error messages
## Usage Examples
### Multi-Architecture Deployment
```yaml
# AMD64 Hardware
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: x86-server
spec:
  interfaces:
    - isoboot:
        sourceISO: "https://images.example.com/hook-amd64.iso"
# ARM64 Hardware
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: arm-server
spec:
  interfaces:
    - isoboot:
        sourceISO: "https://images.example.com/hook-arm64.iso"
### Dynamic Override Example
```bash
# Boot with custom ISO via curl
curl "http://tinkerbell/iso/52:54:00:12:34:56/hook.iso?sourceISO=https://custom.example.com/special.iso"
### Custom Boot Parameters
```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: custom-boot
spec:
  interfaces:
    - isoboot:
        sourceISO: "https://images.example.com/hook.iso"