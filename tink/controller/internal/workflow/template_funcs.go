package workflow

import (
	"fmt"
	"strings"
)

// templateFuncs defines the custom functions available to workflow templates.
var templateFuncs = map[string]interface{}{
	"formatPartition": formatPartition,
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
