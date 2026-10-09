# Hardware templating (v1alpha1)

This guide shows how to enable Go template rendering in v1alpha1 Hardware objects.
Rendering is disabled by default for compatibility with existing payloads.

## Enable rendering

For a Tinkerbell process using the Kubernetes backend, set:

```text
--backend-kube-rendering-enabled
```

With the Helm chart, set:

```yaml
deployment:
  envs:
    globals:
      backendKubeRenderingEnabled: true
```

The backend renders Hardware in the background. Smee, Tootles, Second Star and
Workflow templates (`.hardware` and `.Hardware`) see the rendered copy. Tink Server,
Rufio and the UI see the stored Hardware. The stored Hardware object is not modified.

## Template data

Templates can read the Hardware being rendered as `.hardware` and objects declared
in `spec.references` as `.references.<name>`. A template can also read another
templated field on the same Hardware; dependent fields see its rendered value.
For example, this Hardware reads a domain from a ConfigMap:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: edge-01
  namespace: tinkerbell
spec:
  references:
    site:
      name: site-config
      namespace: tinkerbell
      resource: configmaps
      version: v1
  metadata:
    instance:
      hostname: '{{ .hardware.metadata.name }}.{{ .references.site.data.domain }}'
```

References are denied by default. Configure a reference allow rule and grant the
Tinkerbell service account Kubernetes `get`, `list`, and `watch` permissions for
the referenced resource. The Helm chart's rule settings and RBAC configuration
are described in [Hardware References](REFERENCES.md).

## Template syntax and limits

Only string values under `spec` are rendered, and each result remains a string.
Templates use Go `text/template` syntax. Dot selectors work with identifier-style
field and map keys; use `index` for keys containing punctuation, or to traverse
maps and slices explicitly. For example, this reads hyphenated keys from a reference
named `site-config`:

```gotemplate
{{ index .references "site-config" "data" "domain-name" }}
```

The function library includes Sprig's hermetic functions and Tinkerbell helpers
such as `toYaml`, `fromYaml`, `formatPartition`, and `netmaskToPrefixLength`.
Host- and environment-dependent functions such as `env`, `expandenv`, and
`getHostByName` are not available.

Conditionals and loops can build text inside a string value, but cannot add,
remove, or select YAML fields or list entries. The object structure must already
be valid when Kubernetes admits it. CRD validation also applies to the raw
template text, so a template in a field with a pattern or enum may be rejected
before rendering.

## Preserve literal template syntax

Existing `userData` or `vendorData` may contain cloud-init Jinja such as
`{{ ds.meta_data.hostname }}`. Before enabling rendering, exclude those exact
fields with the `tinkerbell.org/render-skip` annotation:

```yaml
metadata:
  annotations:
    tinkerbell.org/render-skip: '["spec.userData", "spec.vendorData"]'
```

The annotation value is a JSON array of exact string-field paths under `spec`.
Alternatively, write a literal `{{` in a Go template as `{{ "{{" }}`.

## Rendering limits and failures

Metadata, status, and `spec.references` are readable inputs but are not rendered.
Lookup fields also remain literal: `spec.agentID`,
`spec.metadata.instance.id`, `spec.interfaces[].dhcp.mac`, and
`spec.interfaces[].dhcp.ip.address`.

A Hardware with templates is unavailable until it renders. If rendering fails,
Tinkerbell logs the error and the Hardware is treated as not found until it renders
again: Smee, Tootles and Second Star treat it as unknown Hardware, and Workflows retry.
While an edit is still rendering, the previous successful rendering is served.
Check the Tinkerbell logs when a Hardware does not become available.

For the complete field and protection rules, see the
[Hardware templating design](designs/HARDWARE_TEMPLATING.md#52-what-is-rendered).
