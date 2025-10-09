package workflow

import (
	"testing"
)

func TestFormatPartition(t *testing.T) {
	tests := []struct {
		dev       string
		partition int
		expect    string
	}{
		{"/dev/disk/by-id/foobar", 1, "/dev/disk/by-id/foobar-part1"},
		{"/dev/disk/other", 2, "/dev/disk/other-part2"},
		{"/dev/nvme0n1", 1, "/dev/nvme0n1p1"},
		{"/dev/nvme0n1", 5, "/dev/nvme0n1p5"},
		{"/dev/sda", 1, "/dev/sda1"},
		{"/dev/sda", 2, "/dev/sda2"},
		{"/dev/loop0", 3, "/dev/loop0p3"},
		{"/dev/loop", 4, "/dev/loop4"},
	}
	for _, tt := range tests {
		got := formatPartition(tt.dev, tt.partition)
		if got != tt.expect {
			t.Errorf("formatPartition(%q, %d) = %q, want %q", tt.dev, tt.partition, got, tt.expect)
		}
	}
}
