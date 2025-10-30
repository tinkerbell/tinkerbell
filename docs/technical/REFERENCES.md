# Hardware References

This doc will explain what Hardware references are, how to define them, how to use them in a Template, and how to configure access to them.

## What are References?

References are a way to link a Hardware resource to other Kubernetes objects. Once these links are established the fields in the referenced objects are accessible for use in templating a Tinkerbell Template.

### Why is this useful?

This opens up Tinkerbell to integrate with any Kubernetes object and data available. You can create CRDs for things like LVM configurations, network bonding setups, and more.

## How to define References

Here's an example of a reference for a fictional CRD, `lvm.example.org` inside of a Hardware object. All fields are required, except for `group`, which is optional for some resources like `pods`, for example. The `resource` and `group` values should generally always be lowercase. The `resource` field must be the plural version of the object. There is a helper script, [here](../../script/reference_format.sh), for getting this info from a cluster. The string name under `spec.references` can be mostly anything and will be used to reference the object in the Template.

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

## Configuring Access to References

### Access Control

By default, all access to References is denied. The deny all by default is a security feature to limit what can be accessed as the Tink Controller might have cluster wide access. Tink Controller is responsible for Reference lookups, so the access Tink Controller has is the upper bound for Reference access.

### Events, Rules, and Patterns

Tinkerbell uses the Quamina library for handling both the allow and deny list. Quamina's pattern matching syntax and semantics are used to define the rules. We recommend reading the [Quamina documentation](https://github.com/timbray/quamina). Rules are JSON Objects. The data with which the rules are compared are what Quamina calls an "event". The following is the specification of the event that is passed to Quamina for matching against rules.

#### Events

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

The `source` object refers to the Hardware object where the reference is defined. The `name` field is the name of the Hardware object. The `namespace` field is the namespace of the Hardware object. The `reference` object refers to a single referenced object. The `name` field is the name of the referenced object. The `namespace` field is the namespace of the referenced object. The `group` field is the group of the referenced object. The `version` field is the version of the referenced object. The `resource` field is the resource of the referenced object.

The following is an example event.

```json
{
  "source":
    {
      "name":"myhardware",
      "namespace":"tink-system"
    },
  "reference":
    {
      "namespace":"example",
      "name":"exampleLVM",
      "group":"example.org",
      "version":"v1alpha1",
      "resource":"lvms"
    }
}
```

#### Rules and Patterns

Rules are defined in JSON format. Each rule must contains at least 1 pattern. A pattern is a JSON object that follows the syntax and semantics defined by [Quamina](https://github.com/timbray/quamina/blob/main/PATTERNS.md). Multiple patterns are separated by a comma (`,`). All patterns in a single rule are combined with an AND operation. Multiple rules are separated by a pipe (`|`). All rules are combined with an OR operation. By default all patterns are case-sensitive. There is a case-insensitive pattern, see the [Quamina documentation](https://github.com/timbray/quamina/blob/0526acc321a81d4df535caf790879648ace11c86/PATTERNS.md#equals-ignore-case-pattern) for details.

#### Examples

All examples use the example event above. The examples are meant to provide an understanding of rule and pattern construction. For advanced rule and pattern construction, please refer to the [Quamina documentation](https://github.com/timbray/quamina)

> [!NOTE]  
> These are only examples. We recommend building production rules with caution and consideration.

##### Single rule, single pattern

These examples are a single rule with a single pattern.

- This rule allows access for all `Hardware` objects in the `tink-system` namespace to any referenced object in any namespace.

  ```json
  {"source":{"namespace":["tink-system"]}}
  ```

- This rule allows access for all `Hardware` objects, in any namespace to all `lvms` resources, in any namespace.

  ```json
  {"reference":{"resource":["lvms"]}}
  ```

##### Single rule, multiple pattern

These examples are single rules with multiple patterns.

- This rule allows access for all `Hardware` objects in the `tink-system` namespace to all `lvms` objects, in any namespace.

  ```json
  {"source":{"namespace":["tink-system"]},"reference":{"resource":["lvms"]}}
  ```

- This rule allows access for the `example1` `Hardware` object in the `tink-system` namespace to only the `exampleLVM` `lvms` object in the `example` namespace.

  ```json
  {"source":{"name":["example1"],"namespace":["tink-system"]},"reference":{"name":["exampleLVM"],"namespace":["example"],"resource":["lvms"]}} 
  ```

##### Multiple rules

These examples are multiple rules.

- This rule allows access for all `Hardware` objects in the `tink-system` to all `lvms` resources in any namespace OR for all `Hardware` objects in the `tink-system` namespace to all `bonds` resources in any namespace.

  ```json
  {"source":{"namespace":["tink-system"]},"reference":{"resource":["lvms"]}}|{"source":{"namespace":["tink-system"]},"reference":{"resource":["bonds"]}}
  ```

### Configuring Access

Use the CLI flags or environment variables to define both the allow and deny rules. The allow list takes precedence over the deny list.

> [!NOTE]  
> If a deny rule is defined, it will override the default deny all access.

CLI Flags:

- `--tink-controller-reference-allow-list-rules`
- `--tink-controller-reference-deny-list-rules`

```bash
--tink-controller-reference-allow-list-rules='{"reference":{"resource":["hardware"]}}|{"reference":{"resource":["workflows"]}}'
```

Environment Variables:

- `TINKERBELL_TINK_CONTROLLER_REFERENCE_ALLOW_LIST_RULES`
- `TINKERBELL_TINK_CONTROLLER_REFERENCE_DENY_LIST_RULES`

```bash
export TINKERBELL_TINK_CONTROLLER_REFERENCE_ALLOW_LIST_RULES='{"reference":{"resource":["hardware"]}}|{"reference":{"resource":["workflows"]}}'
```
