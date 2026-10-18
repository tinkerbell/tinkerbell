{{/*
Report whether IPv4 listeners are served, matching how the binary resolves
listenerFamilies. Returns "true" or an empty string, so the result works
directly in an if.
Usage: {{ if include "tinkerbell.servesIPv4" . }}
*/}}
{{- define "tinkerbell.servesIPv4" -}}
{{- if has .Values.deployment.envs.globals.listenerFamilies (list "ipv4" "dual") -}}true{{- end -}}
{{- end -}}

{{/*
Report whether IPv6 listeners are served.
Usage: {{ if include "tinkerbell.servesIPv6" . }}
*/}}
{{- define "tinkerbell.servesIPv6" -}}
{{- if has .Values.deployment.envs.globals.listenerFamilies (list "ipv6" "dual") -}}true{{- end -}}
{{- end -}}

{{/*
The port a container actually binds for a setting. Only IPv6 being served makes
the IPv6 value the live one; otherwise the IPv4 value is, because a Service port
has a single targetPort that both families share.
Usage: {{ include "tinkerbell.servedPort" (dict "v4" $httpPort "v6" $httpPortV6 "ctx" $) }}
*/}}
{{- define "tinkerbell.servedPort" -}}
{{- if and (include "tinkerbell.servesIPv6" .ctx) (not (include "tinkerbell.servesIPv4" .ctx)) -}}
{{- .v6 -}}
{{- else -}}
{{- .v4 -}}
{{- end -}}
{{- end -}}

{{/*
The namespaces Tinkerbell watches, as a JSON array. rbac.type selects the profile:
with Role, the release namespace plus rbac.namespaces; with ClusterRole, empty (all
namespaces). The deprecated deployment.envs.globals.backendKubeNamespace counts as
one more entry in rbac.namespaces, and with ClusterRole limits Tinkerbell to it.
Usage: {{ include "tinkerbell.watchedNamespaces" . | fromJsonArray }}
*/}}
{{- define "tinkerbell.watchedNamespaces" -}}
{{- $listed := default (list) .Values.rbac.namespaces -}}
{{- if not (kindIs "slice" $listed) -}}
{{- fail "rbac.namespaces must be a list of namespace names" -}}
{{- end -}}
{{- $legacy := .Values.deployment.envs.globals.backendKubeNamespace -}}
{{- range $name := concat $listed (ternary (list $legacy) (list) (not (empty $legacy))) -}}
{{- if not (and (kindIs "string" $name) (le (len $name) 63) (regexMatch "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" $name)) -}}
{{- fail (printf "invalid namespace %q in rbac.namespaces or deployment.envs.globals.backendKubeNamespace: must be one lowercase DNS-1123 label (for several namespaces, list them in rbac.namespaces)" (toString $name)) -}}
{{- end -}}
{{- end -}}
{{- $namespaces := list -}}
{{- if eq .Values.rbac.type "Role" -}}
{{- $namespaces = concat (list .Release.Namespace) $listed -}}
{{- else if $listed -}}
{{- fail "rbac.namespaces requires rbac.type Role: a ClusterRole already grants every namespace. To limit a ClusterRole to one namespace, set deployment.envs.globals.backendKubeNamespace instead" -}}
{{- end -}}
{{- if $legacy -}}
{{- $namespaces = append $namespaces $legacy -}}
{{- end -}}
{{- toJson (uniq $namespaces) -}}
{{- end -}}

{{/*
The backend namespaces as the comma-separated TINKERBELL_BACKEND_KUBE_NAMESPACE value.
Empty means all namespaces.
Usage: {{ include "tinkerbell.backendNamespaces" . }}
*/}}
{{- define "tinkerbell.backendNamespaces" -}}
{{- include "tinkerbell.watchedNamespaces" . | fromJsonArray | join "," -}}
{{- end -}}

{{/*
The namespaces that get the Role/ClusterRole and binding objects, as a JSON array:
every watched namespace with Role, or one cluster-scoped entry ("") with ClusterRole.
Usage: {{ include "tinkerbell.roleNamespaces" . | fromJsonArray }}
*/}}
{{- define "tinkerbell.roleNamespaces" -}}
{{- if eq .Values.rbac.type "Role" -}}
{{- include "tinkerbell.watchedNamespaces" . -}}
{{- else -}}
{{- toJson (list "") -}}
{{- end -}}
{{- end -}}

{{/*
The one namespace for settings that take a single namespace (auto-discovered
Hardware, leader-election Leases and their Role): the watched namespace when there
is exactly one, otherwise the release namespace. Tinkerbell makes the same choice.
Usage: {{ include "tinkerbell.primaryNamespace" . }}
*/}}
{{- define "tinkerbell.primaryNamespace" -}}
{{- $namespaces := include "tinkerbell.watchedNamespaces" . | fromJsonArray -}}
{{- if eq (len $namespaces) 1 -}}
{{- first $namespaces -}}
{{- else -}}
{{- .Release.Namespace -}}
{{- end -}}
{{- end -}}

{{/*
enableCRDMigrations, or the profile default when empty or null: on with ClusterRole,
off with Role, which cannot grant access to cluster-scoped CRDs.
Usage: {{ include "tinkerbell.enableCRDMigrations" . }}
*/}}
{{- define "tinkerbell.enableCRDMigrations" -}}
{{- $value := .Values.deployment.envs.globals.enableCRDMigrations -}}
{{- if or (kindIs "invalid" $value) (eq (toString $value) "") -}}
{{- eq .Values.rbac.type "ClusterRole" -}}
{{- else -}}
{{- $value -}}
{{- end -}}
{{- end -}}

{{/*
Ensure a value is a JSON array. Fails on nil or non-array input.
- slice/array: use as-is
- nil/missing: fail with an error (required field)
- anything else: fail with a type error
Usage: {{ include "tinkerbell.toJsonArray" .apiGroups }}
*/}}
{{- define "tinkerbell.toJsonArray" -}}
{{- if kindIs "slice" . -}}
  {{- . | toJson -}}
{{- else if kindIs "invalid" . -}}
  {{- fail "required field is nil/missing in rbac.additionalRoleRules entry (apiGroups, resources, and verbs are required and must be arrays)" -}}
{{- else -}}
  {{- fail (printf "expected an array but got %s: %v" (kindOf .) .) -}}
{{- end -}}
{{- end -}}

{{/*
Render an optional RBAC field as a JSON array. Skips nil/missing values.
- slice/array: render as JSON array
- nil/missing: output nothing (caller uses 'with' or checks result)
- anything else: fail with a type error
Usage: {{ include "tinkerbell.toOptionalJsonArray" .resourceNames }}
*/}}
{{- define "tinkerbell.toOptionalJsonArray" -}}
{{- if kindIs "slice" . -}}
  {{- . | toJson -}}
{{- else if not (kindIs "invalid" .) -}}
  {{- fail (printf "expected an array but got %s: %v" (kindOf .) .) -}}
{{- end -}}
{{- end -}}

