{{- define "stack.hookDownloadJobEnabled" -}}
{{- $result := .Values.stack.persistence.enabled }}
{{- if and .Values.stack.persistence.enabled (eq .Values.stack.persistence.type "pvc") }}
  {{- range .Values.stack.persistence.accessModes }}
    {{- if eq . "ReadWriteOnce" }}
      {{- $result = false}}
    {{- end }}
  {{- end }}
{{- end }}
{{- $result }}
{{- end -}}