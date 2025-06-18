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

To connect to the serial-over-ssh console, ssh to the Tinkerbell IP (`kubectl get svc -n tinkerbell tinkerbell -o jsonpath='{.status.loadBalancer.ingress[0].ip}'`) using the Hardware object's `spec.metadata.name` as the user and with the `-p 2222` option.

```bash
ssh -p 2222 example-hardware@192.168.2.50
```

## Host key

### What is a host key?

A host key is a cryptographic key used to verify the identity of a server when connecting via SSH.
It ensures that the client is connecting to the correct server and not an imposter.



If a host key is not provided one will be generated on the fly. This is not recommended for production use, as it can lead to security issues. To provide a host key, create a Kubernetes secret in the `tinkerbell` namespace with the name `secondstar-hostkey` and the key `hostkey`. The value should be the private key in OpenSSH format.

```bash
kubectl create secret generic secondstar-hostkey \
  --namespace tinkerbell \
  --from-file=hostkey=/path/to/your/private/key
```

Mount the secret in the Second Star deployment by specifying a volume mount in the Helm chart:

```bash
--set "deployment.volumes[0].name=secondstar-hostkey" \
--set "deployment.volumes[0].secret.secretName=secondstar-hostkey"
```

And then mount it in the container by specifying the mount path in the Helm chart:

```bash
--set "deployment.volumeMounts[0].name=secondstar-hostkey" \
--set "deployment.volumeMounts[0].mountPath=/etc/ssh/secondstar_hostkey" \
--set "deployment.volumeMounts[0].subPath=hostkey" \
--set "deployment.volumeMounts[0].readOnly=true"
```

Finally, set the Second Star host key environment variable in the Second Star deployment to point to the mounted host key:

```bash
--set "deployment.envs.secondstar.hostKeyPath=/etc/ssh/secondstar_hostkey"
```
