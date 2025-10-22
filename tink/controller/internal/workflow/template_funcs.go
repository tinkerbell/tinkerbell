package workflow

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// templateFuncs defines the custom functions available to workflow templates.
var templateFuncs = map[string]interface{}{
	"formatPartition":       formatPartition,
	"netmaskToPrefixLength": netmaskToPrefixLength,
}

// formatPartition formats a device path with partition for the device type. If it receives an
// unidentifiable device path it returns the dev.
//
// Examples
//
//	formatPartition("/dev/nvme0n1", 1) -> /dev/nvme0n1p1
//	formatPartition("/dev/sda", 1) -> /dev/sda1
//	formatPartition("/dev/vda", 2) -> /dev/vda2
func formatPartition(dev string, partition int) string {
	switch {
	case strings.HasPrefix(dev, "/dev/nvme"):
		return fmt.Sprintf("%vp%v", dev, partition)
	case strings.HasPrefix(dev, "/dev/sd"),
		strings.HasPrefix(dev, "/dev/vd"),
		strings.HasPrefix(dev, "/dev/xvd"),
		strings.HasPrefix(dev, "/dev/hd"):
		return fmt.Sprintf("%v%v", dev, partition)
	}
	return dev
}

// netmaskToPrefixLength converts a netmask (e.g. 255.255.255.0) to prefix length (e.g. 24).
// Returns an error if the netmask is invalid or not IPv4.
func netmaskToPrefixLength(netmask string) (string, error) {
	// Parse the netmask
	ip := net.ParseIP(netmask)
	if ip == nil {
		return "", fmt.Errorf("invalid netmask format: %s", netmask)
	}

	// Convert to IPv4
	ipv4 := ip.To4()
	if ipv4 == nil {
		return "", fmt.Errorf("netmask must be IPv4: %s", netmask)
	}

	// Count the number of 1 bits in the netmask
	ones, _ := net.IPMask(ipv4).Size()
	return strconv.Itoa(ones), nil
}
