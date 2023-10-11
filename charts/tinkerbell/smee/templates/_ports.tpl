{{ define "smee.ports" }}
- {{ .PortKey }}: {{ .http.port }}
  name: {{ .http.name }}
  protocol: TCP
- {{ .PortKey }}: {{ .syslog.port }}
  name: {{ .syslog.name }}
  protocol: UDP
- {{ .PortKey }}: {{ .dhcp.port }}
  name: {{ .dhcp.name }}
  protocol: UDP
- {{ .PortKey }}: {{ .tftp.port }}
  name: {{ .tftp.name }}
  protocol: UDP
{{- end }}

{{- define "urlJoiner" }}
{{- $host := printf "%v:%v" .urlDict.host .urlDict.port }}
{{- $newDict := set .urlDict "host" $host }}
{{- print (urlJoin $newDict) }}
{{- end }}