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

// formatPartition formats a device path with partition for the device type.
// It will never return just the dev.
// if dev has prefix "/dev/disk/", then partitions are always suffixed "-partX" no matter the device type.
// otherwise, if dev ends in a digit, then partitions are suffixed with "pX" (e.g. /dev/nvme0n1 -> /dev/nvme0n1p1).
// otherwise, partitions are suffixed with "X" (e.g. /dev/sda -> /dev/sda1).
func formatPartition(dev string, partition int) string {
	if strings.HasPrefix(dev, "/dev/disk/") {
		return fmt.Sprintf("%v-part%v", dev, partition)
	}
	if len(dev) > 0 && dev[len(dev)-1] >= '0' && dev[len(dev)-1] <= '9' {
		return fmt.Sprintf("%vp%v", dev, partition)
	}
	return fmt.Sprintf("%v%v", dev, partition)
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
