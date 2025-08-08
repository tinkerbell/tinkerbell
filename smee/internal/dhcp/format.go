package dhcp

import "net"

const hexDigit = "0123456789abcdef"

// dashNotation formats a net.HardwareAddr into its dash notation string.
//
// 00-00-5e-00-53-01
// 02-00-5e-10-00-00-00-01
// 00-00-00-00-fe-80-00-00-00-00-00-00-02-00-5e-10-00-00-00-01
func dashNotation(a net.HardwareAddr) string {
	if len(a) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(a)*3-1)
	for i, b := range a {
		if i > 0 {
			buf = append(buf, '-')
		}
		buf = append(buf, hexDigit[b>>4])
		buf = append(buf, hexDigit[b&0xF])
	}
	return string(buf)
}

// dotNotation formats a net.HardwareAddr into its dot notation string.
//
// 0000.5e00.5301
// 0200.5e10.0000.0001
// 0000.0000.fe80.0000.0000.0000.0200.5e10.0000.0001
func dotNotation(a net.HardwareAddr) string {
	if len(a) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(a)*5-1)
	for i, b := range a {
		if i > 0 && i%2 == 0 {
			buf = append(buf, '.')
		}
		buf = append(buf, hexDigit[b>>4])
		buf = append(buf, hexDigit[b&0xF])
	}
	return string(buf)
}

// noDelimiter formats a net.HardwareAddr into a string without any delimiters.
//
// 00005e005301
// 02005e1000000001
// 00000000fe80000000000002005e1000000001
func noDelimiter(a net.HardwareAddr) string {
	if len(a) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(a)*2)
	for _, b := range a {
		buf = append(buf, hexDigit[b>>4])
		buf = append(buf, hexDigit[b&0xF])
	}
	return string(buf)
}
