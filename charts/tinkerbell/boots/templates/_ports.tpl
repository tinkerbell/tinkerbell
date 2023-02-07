{{ define "boots.ports" }}
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