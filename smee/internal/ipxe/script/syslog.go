package script

// syslogScript resolves the selected hostname on the booting client each time
// the script runs. Keep the DNS result untyped so set can reject an answer from
// the wrong address family without changing either syslog setting.
const syslogScript = `
{{- if .SelectedSyslogHost }}
# Try an IP literal first so it works even when nslookup is unavailable.
# Leave the nslookup destination untyped to preserve the resolved address's type.
clear syslog-address
{{- if eq .AddressFamily "ipv6" }}
set syslog-address:ipv6 {{ .SyslogHostV6 }} || nslookup syslog-address {{ .SyslogHostV6 }} && set syslog6 ${syslog-address} || echo [WARN] Failed to configure syslog6 host {{ .SyslogHostV6 }}: resolution failed or expected ipv6 address
{{- else }}
set syslog-address:ipv4 {{ .SyslogHost }} || nslookup syslog-address {{ .SyslogHost }} && set syslog ${syslog-address} || echo [WARN] Failed to configure syslog host {{ .SyslogHost }}: resolution failed or expected ipv4 address
{{- end }}
clear syslog-address
{{- end}}
`

// SelectedSyslogHost returns the host for the boot script's address family.
func (h Hook) SelectedSyslogHost() string {
	if h.AddressFamily == ipv6 {
		return h.SyslogHostV6
	}
	return h.SyslogHost
}
