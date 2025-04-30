# Hardware References

This doc will explain what Hardware references are, how to define them, and how to use them in a Template.

## What are References?

The Hardware custom resource defines a field, `spec.references`, which allows for 

The string name under `spec.reference` can be anything and will be used to reference the object in the Template. The name is not required to be the same as the name of the object being referenced.

The `resource` field must be all lowercase and the plural version of the object.

## How to define References

Here's an example of referencing a CRD for LVM data.

```yaml
spec:
  references:
    lvm:
      group: example.org
      name: lvm1
      namespace: tink
      resource: lvms
      version: v1
```

or an example of referencing a CRD for network bonding data.

```yaml
spec:
  references:
    bonding:
      group: example.org
      name: bond1
      namespace: tink
      resource: bonds
      version: v1
```

Here's an example to reference the same Hardware object in which the references are defined.

```yaml
spec:
  references:
    hw:
      group: tinkerbell.org
      name: virtual
      namespace: tink
      resource: hardware
      version: v1alpha1
```

## How to use References

The following shows how to use references in a Template. Start with `.reference`, then add the name of the reference `.references.<name>`, and then the field you want to access. For example, to access the `group` field of the `lvm` reference, you would use `.references.lvm.spec.group`.

```yaml
spec:
  data: |
    name: Example
    global_timeout: 600
    tasks:
      - name: "example Task"
        worker: "{{.worker_id}}"
        actions:
          - name: "example Action one"
            image: alpine
            timeout: 60
            command: ["echo", "{{ .references.lvm.spec.group }}"]
          - name: "example Action two"
            image: alpine
            timeout: 60
            command: ["echo", "{{ .references.bonding.spec.members }}"]
          - name: "example Action three"
            image: alpine
            timeout: 60
            command: ["echo", "{{ .references.hw.spec.userData }}"]
```

## Tink Controller Flags

By default, Tink Controller is configured to deny all Reference objects. This is a security feature to limit what can be accessed. While the Tink Controller might have cluster wide access to lots of namespaces and resource types, the Hardware References don't by default have the same access. In order to allow access to a specific resource, the Tink Controller must be configured with the CLI flag, `--tink-controller-reference-allow-list-rules`. There is also a CLI flag, `--tink-controller-reference-deny-list-rules`, which will deny access to a specific resource. The allow list takes precedence over the deny list. The allow list is a list of rules that define what resources are allowed to be accessed by the Hardware References. The deny list is a list of rules that define what resources are denied access by the Hardware References.

example of specifying multiple allow rules.

```bash
# This is two rules that will be "or"ed together
--tink-controller-reference-allow-list-rules '{"resource":["hardware"]} {"resource": ["workflows"]}'

# This is one rule that must match all patterns. Each patter is "and"ed together
--tink-controller-reference-allow-list-rules '{"resource":["hardware"],"namespace":["tink-system"]}'
```

