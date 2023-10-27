{{- define "stack.hookDownloadJobEnabled" -}}
  {{- $result := false }}
  {{- if and .Values.stack.hook.enabled .Values.stack.hook.persistence.enabled -}}
    {{- $result = true -}}
    {{- if and .Values.stack.hook.persistence.enabled (eq .Values.stack.hook.persistence.type "pvc") }}
      {{- range .Values.stack.hook.persistence.accessModes }}
        {{- if eq . "ReadWriteOnce" }}
          {{- $result = false}}
        {{- end }}
      {{- end }}
    {{- end }}
  {{- end -}}
  {{- $result }}
{{- end -}}