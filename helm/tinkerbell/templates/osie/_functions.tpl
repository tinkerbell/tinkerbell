{{/*
Test if the given value is an IP address
{{ include "tinkerbell.isIpAddress" "1.2.3.4" }}
*/}}
{{- define "tinkerbell.isIpAddress" -}}
{{- $ipv4Pattern := "^((25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])\\.){3}(25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])$" -}}
{{- $isIPv6 := and (contains ":" .) (regexMatch "^[0-9A-Fa-f:.]+$" .) -}}
{{- if or (regexMatch $ipv4Pattern .) $isIPv6 -}}
{{- . -}}
{{- end -}}
{{- end -}}

{{/*
Test if the given value is an IPv4 address
{{ include "tinkerbell.isIPv4Address" "1.2.3.4" }}
*/}}
{{- define "tinkerbell.isIPv4Address" -}}
{{- $ipv4Pattern := "^((25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])\\.){3}(25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])$" -}}
{{- if regexMatch $ipv4Pattern . -}}
{{- . -}}
{{- end -}}
{{- end -}}

{{/*
Test if the given value is an IPv6 address
{{ include "tinkerbell.isIPv6Address" "2001:db8::10" }}
*/}}
{{- define "tinkerbell.isIPv6Address" -}}
{{- $value := trimAll "[]" . -}}
{{- if and (regexMatch ".*:.*:.*" $value) (regexMatch "^[0-9A-Fa-f:.]+$" $value) -}}
{{- $value -}}
{{- end -}}
{{- end -}}

{{/*
Extract the hostname or IP literal from a parsed URL host.
{{ include "tinkerbell.urlHost" "192.0.2.10:7173" }}
{{ include "tinkerbell.urlHost" "[2001:db8::10]:7173" }}
*/}}
{{- define "tinkerbell.urlHost" -}}
{{- if hasPrefix "[" . -}}
{{- regexFind "\\[[^]]+\\]" . | trimAll "[]" -}}
{{- else if regexMatch ".*:.*:.*" . -}}
{{- . -}}
{{- else -}}
{{- regexFind "^[^:]+" . -}}
{{- end -}}
{{- end -}}

{{/*
osie.* helpers resolve shared OSIE (Operating System Installation Environment) configuration.
They check optional.hookos first (backward compatibility) and fall back to optional.osie.
This allows existing users with hookos overrides to keep working while new installs use osie.

Nil detection uses "kindIs invalid" to correctly handle YAML null values (from deprecated
hookos placeholders like "image:") while preserving explicitly set values including false booleans.
*/}}

{{/* Scalar: returns hookos value if non-nil, else osie */}}
{{- define "tinkerbell.osie.name" -}}
{{- if not (kindIs "invalid" .Values.optional.hookos.name) -}}
{{- .Values.optional.hookos.name -}}
{{- else -}}
{{- .Values.optional.osie.name -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.image" -}}
{{- if not (kindIs "invalid" .Values.optional.hookos.image) -}}
{{- .Values.optional.hookos.image -}}
{{- else -}}
{{- .Values.optional.osie.image -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.port" -}}
{{- if not (kindIs "invalid" .Values.optional.hookos.port) -}}
{{- .Values.optional.hookos.port -}}
{{- else -}}
{{- .Values.optional.osie.port -}}
{{- end -}}
{{- end -}}

{{/* Bool: kindIs "invalid" distinguishes nil from false */}}
{{- define "tinkerbell.osie.hostNetwork" -}}
{{- if not (kindIs "invalid" .Values.optional.hookos.hostNetwork) -}}
{{- .Values.optional.hookos.hostNetwork -}}
{{- else -}}
{{- .Values.optional.osie.hostNetwork -}}
{{- end -}}
{{- end -}}

{{/* Nested scalar: use dig with nil default */}}
{{- define "tinkerbell.osie.deploymentStrategy" -}}
{{- $val := dig "hookos" "deployment" "strategy" "type" nil .Values.optional -}}
{{- if not (kindIs "invalid" $val) -}}
{{- $val -}}
{{- else -}}
{{- .Values.optional.osie.deployment.strategy.type -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.service.type" -}}
{{- $val := dig "hookos" "service" "type" nil .Values.optional -}}
{{- if not (kindIs "invalid" $val) -}}
{{- $val -}}
{{- else -}}
{{- .Values.optional.osie.service.type -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.service.lbClass" -}}
{{- $val := dig "hookos" "service" "lbClass" nil .Values.optional -}}
{{- if not (kindIs "invalid" $val) -}}
{{- $val -}}
{{- else -}}
{{- .Values.optional.osie.service.lbClass -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.service.loadBalancerIP" -}}
{{- $val := dig "hookos" "service" "loadBalancerIP" nil .Values.optional -}}
{{- if not (kindIs "invalid" $val) -}}
{{- $val -}}
{{- else -}}
{{- .Values.optional.osie.service.loadBalancerIP -}}
{{- end -}}
{{- end -}}

{{/*
Collection helpers return YAML text. Returns empty string when both hookos and osie values are empty.
Selector requires special handling: deprecated hookos.selector is {app: null}, so we check
if at least one map value is non-nil before using it.
*/}}
{{- define "tinkerbell.osie.selector" -}}
{{- $use := false -}}
{{- if not (kindIs "invalid" .Values.optional.hookos.selector) -}}
  {{- range $_, $v := .Values.optional.hookos.selector -}}
    {{- if not (kindIs "invalid" $v) -}}
      {{- $use = true -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- if $use -}}
{{- .Values.optional.hookos.selector | toYaml -}}
{{- else -}}
{{- .Values.optional.osie.selector | toYaml -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.nodeSelector" -}}
{{- if and (not (kindIs "invalid" .Values.optional.hookos.nodeSelector)) (gt (len .Values.optional.hookos.nodeSelector) 0) -}}
{{- .Values.optional.hookos.nodeSelector | toYaml -}}
{{- else if gt (len .Values.optional.osie.nodeSelector) 0 -}}
{{- .Values.optional.osie.nodeSelector | toYaml -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.tolerations" -}}
{{- if and (not (kindIs "invalid" .Values.optional.hookos.tolerations)) (gt (len .Values.optional.hookos.tolerations) 0) -}}
{{- .Values.optional.hookos.tolerations | toYaml -}}
{{- else if gt (len .Values.optional.osie.tolerations) 0 -}}
{{- .Values.optional.osie.tolerations | toYaml -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.affinity" -}}
{{- if and (not (kindIs "invalid" .Values.optional.hookos.affinity)) (gt (len .Values.optional.hookos.affinity) 0) -}}
{{- .Values.optional.hookos.affinity | toYaml -}}
{{- else if gt (len .Values.optional.osie.affinity) 0 -}}
{{- .Values.optional.osie.affinity | toYaml -}}
{{- end -}}
{{- end -}}

{{- define "tinkerbell.osie.service.annotations" -}}
{{- $hookAnn := dig "hookos" "service" "annotations" nil .Values.optional -}}
{{- if and (not (kindIs "invalid" $hookAnn)) (gt (len $hookAnn) 0) -}}
{{- $hookAnn | toYaml -}}
{{- else if gt (len .Values.optional.osie.service.annotations) 0 -}}
{{- .Values.optional.osie.service.annotations | toYaml -}}
{{- end -}}
{{- end -}}
