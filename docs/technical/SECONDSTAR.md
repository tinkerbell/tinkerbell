# Second Star

Second Star is the serial over SSH capability in Tinkerbell.
It is an SSH wrapper over the `ipmitool sol activate` command. The SOL (serial-over-lan) command connects to a Hardware BMC's (Baseboard Management Controller) serial console using the ipmi protocol.

at least one ssh public key must be in the Hardware object at `spec.metadata.instance.ssh_keys`.

```yaml
spec:
  metadata:
    instance:
      ssh_keys:
        - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3...
```

A `bmcRef` must be defined in the Hardware object.

```yaml
spec:
  bmcRef:
    kind: machine.bmc.tinkerbell.org
    name: example-bmc
```

The `bmcRef` itself must be a machine that has ipmi serial-over-lan enabled. See your BMC documentation for details on how to enable this.
The `bmcRef` must also have a `spec.connection.host` and `spec.connection.authSecretRef` defined. The `machine.bmc.tinkerbell.org` object must have a `status.conditions` of `type: Contactable` with a `status: "True"`.

To connect to the serial-over-ssh console, ssh to the Tinkerbell IP using the Hardware object's `spec.metadata.name` as the user and with the `-p 2222` option.

```bash
ssh -p 2222 example-hardware@192.168.2.50
```
