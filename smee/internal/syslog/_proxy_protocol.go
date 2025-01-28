package syslog

import (
	"bufio"
	"bytes"
	"net"
	"strings"
)

var (
	// prefix is the string we look for at the start of a connection
	// to check if this connection is using the proxy protocol.
	prefix    = []byte("PROXY ")
	prefixLen = len(prefix)
)

// extractSrcIP extracts the source IP from the PROXY protocol header
// https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
func extractSrcIP(b [2048]byte) ([2048]byte, net.IP) {
	buf := make([]byte, len(b))
	copy(buf, b[:])
	bufReader := bufio.NewReader(bytes.NewReader(buf))

	for i := 1; i <= prefixLen; i++ {
		inp, err := bufReader.Peek(i)
		if err != nil {
			return b, nil
		}

		// Check for a prefix mis-match, quit early
		if !bytes.Equal(inp, prefix[:i]) {
			return b, nil
		}
	}

	// Read the header line
	header, err := bufReader.ReadString('\n')
	if err != nil {
		return b, nil
	}

	// Strip the carriage return and new line
	header = header[:len(header)-2]

	// Split on spaces, should be (PROXY <type> <src addr> <dst addr> <src port> <dst port>)
	parts := strings.Split(header, " ")
	if len(parts) < 2 {
		return b, nil
	}

	// Verify the type is known
	switch parts[1] {
	case "UNKNOWN":
		return b, nil
	case "TCP4", "TCP6":
	default:
		return b, nil
	}

	if len(parts) != 6 {
		return b, nil
	}

	// Parse out the source address
	ip := net.ParseIP(parts[2])
	if ip == nil {
		return b, nil
	}

	// remove Proxy Protocol headers.
	idx := bytes.IndexByte(b[:], '\n')
	bg := [2048]byte{}
	copy(bg[:], b[idx+1:])
	// remove NUL chars if present
	bg2 := bytes.Trim(bg[:], "\x00")
	copy(bg[:], bg2)
	bg3 := bytes.Trim(bg2, "\n")
	copy(bg[:], bg3)

	return bg, ip
}
