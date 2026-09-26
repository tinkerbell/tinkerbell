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

{{/*
Report whether the deployment serves IPv6 only. Outputs "true" or "".
Taken from service.ipFamilies, falling back to which public address is set.
Usage: {{ include "tinkerbell.ipv6Only" . }}
*/}}
{{- define "tinkerbell.ipv6Only" -}}
{{- $families := .Values.service.ipFamilies -}}
{{- if $families -}}
  {{- if and (has "IPv6" $families) (not (has "IPv4" $families)) -}}true{{- end -}}
{{- else -}}
  {{- $v4 := coalesce .Values.deployment.envs.globals.publicIpv4 .Values.publicIP -}}
  {{- $v6 := coalesce .Values.deployment.envs.globals.publicIpv6 .Values.publicIPv6 -}}
  {{- if and $v6 (not $v4) -}}true{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Resolve the container port for a service. A Service port entry carries one
targetPort for both families, so an IPv6-only deployment must expose the IPv6
port instead of the IPv4 one.
Usage: {{ include "tinkerbell.containerPort" (dict "v4" 69 "v6" "" "v6only" $v6only) }}
*/}}
{{- define "tinkerbell.containerPort" -}}
{{- if .v6only -}}{{ default .v4 .v6 }}{{- else -}}{{ .v4 }}{{- end -}}
{{- end -}}

