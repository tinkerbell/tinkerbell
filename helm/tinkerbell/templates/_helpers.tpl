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
