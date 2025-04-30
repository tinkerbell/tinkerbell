# Hardware References

This doc will explain what Hardware references are, how to define them, and how to use them in a Template.

## What are References?

The Hardware custom resource defines a field, `spec.references`, which allows for 

The string name under `spec.references` can be anything and will be used to reference the object in the Template. The name is not required to be the same as the name of the object being referenced.

The `resource` field must be all lowercase and the plural version of the object.

## How to define References

Here's an example of referencing a fictional CRD for LVM data. All fields are required, except for `group`, which is optional for some resources like `pods`, for example. There is a script that will retrieve this information from a cluster located [here](../script/reference_format.sh).

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

## Limiting Access to References

By default, Tink Controller is configured to deny access to all Reference objects. This is a security feature to limit what can be accessed as the Tink Controller might have cluster wide access. In order to allow access to a specific resource, the Tink Controller must be configured with the CLI flag, `--tink-controller-reference-allow-list-rules`. There is also a CLI flag, `--tink-controller-reference-deny-list-rules`, which will deny access to a specific resource. The allow list takes precedence over the deny list. The allow list is a list of rules that define what resources are allowed to be accessed by the Hardware References. The deny list is a list of rules that define what resources are denied access by the Hardware References.

Rules JSON Object, this is what Quamina calls an "event".

```json
{
  "source":
    {
      "name":"",
      "namespace":""
    },
  "reference":
    {
      "namespace":"",
      "name":"",
      "group":"",
      "version":"",
      "resource":""
    }
}
```

Rules are defined in JSON format. Each rule contains greater than 0 patterns.

Example of specifying multiple allow rules. Multiple rules are separated by a space.

```bash
# This is two rules that will be "or"ed together. Space delimited.
--tink-controller-reference-allow-list-rules '{"reference":{"resource":["hardware"]}} {"reference":{"resource":["workflows"]}}'

# This is one rule, encapsulated by an outer `{}`, that must match all patterns. All patterns are "and"ed together.
--tink-controller-reference-allow-list-rules '{"reference":{"resource":["hardware"],"source":{"namespace":["tink-system"]}}}'
```

```bash
--tink-controller-reference-allow-list-rules '{"reference":{"resource":["hardware"],"namespace":["tink-system"]},"source":{"namespace":["tink-system"]}} {"reference":{"resource":["pods"]}}'
```
