[![Build Status](https://github.com/tinkerbell/hegel/actions/workflows/ci.yaml/badge.svg)](https://github.com/tinkerbell/hegel/actions/workflows/ci.yaml)

# Hegel

Hegel is an instance metadata service used by Tinkerbell for bare metal instance initialization.

When bare metal machines are provisioned using the Tinkerbell stack they inevitably boot into a
permanent OS. The permanent OS, much like the underlying hardware, needs initializing before it 
can be used. The initialization is commonly performed by tools such as [cloud-init] or [ignition]. 
The configuration used by these processes is provided by an instance metadata source. The source
could be anything but is commonly an HTTP API.

Hegel exposes common instance metadata APIs for your OS intialization needs including AWS EC2 
instance metadata.

## Version Compatibility

We follow semantic versioning and the project is currently v0 meaning compatibility is best effort.
If you have any specific concerns don't hesitate to raise an issue.

[cloud-init]: https://cloudinit.readthedocs.io/en/latest/
[ignition]: https://coreos.github.io/ignition/