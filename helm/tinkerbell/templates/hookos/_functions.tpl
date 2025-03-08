{{/*
Test if the given value is an IP address
{{ include "isIpAddress" "1.2.3.4" }}
*/}}
{{- define "isIpAddress" -}}
{{- $rc := . -}}
{{- $parts := splitList "." . -}}
{{- if eq (len $parts) 4 -}}
    {{- range $parts -}}
        {{- if and (not (atoi .)) (ne . "0") -}}
            {{- $rc = "" -}}
        {{- end -}}
    {{- end -}}
{{- else -}}
    {{- $rc = "" -}}
{{- end -}}
{{- print $rc }}
{{- end -}}
