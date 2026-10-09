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
The backend namespace. rbac.type selects the profile: with Role (namespaced)
it defaults to the release namespace, with ClusterRole to empty (all namespaces).
Usage: {{ include "tinkerbell.backendNamespace" . }}
*/}}
{{- define "tinkerbell.backendNamespace" -}}
{{- if .Values.deployment.envs.globals.backendKubeNamespace -}}
{{- .Values.deployment.envs.globals.backendKubeNamespace -}}
{{- else if eq .Values.rbac.type "Role" -}}
{{- .Release.Namespace -}}
{{- end -}}
{{- end -}}

{{/*
The namespace for the namespaced RBAC objects: the backend namespace, otherwise
the release namespace.
Usage: {{ include "tinkerbell.rbacNamespace" . }}
*/}}
{{- define "tinkerbell.rbacNamespace" -}}
{{- coalesce (include "tinkerbell.backendNamespace" .) .Release.Namespace -}}
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

