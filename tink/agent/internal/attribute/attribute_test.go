package attribute

import (
	"math"
	"testing"
)

func TestParseSpeedMbps(t *testing.T) {
	tests := map[string]uint32{
		"1000":     1000,
		"1000Mb/s": 1000,
		"25000":    25000,
		"":         0,
		"unknown":  0,
		"10 Gb/s":  10000,
		"25 GB/s":  200000,
		"25 MB/s":  200,
		"-1":       0,
		"-100":     0,
	}
	for input, want := range tests {
		if got := parseSpeedMbps(input); got != want {
			t.Errorf("parseSpeedMbps(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseSpeedMbpsOverflowClamps(t *testing.T) {
	// A value whose Gb/s-scaled result exceeds uint32 must clamp, not parse-error
	// to 0: ParseUint(..., 64) succeeds where ParseUint(..., 32) would have
	// returned ErrRange, so the math.MaxUint32 clamp below actually runs.
	if got, want := parseSpeedMbps("5000000 Gb/s"), uint32(math.MaxUint32); got != want {
		t.Errorf("parseSpeedMbps(overflow) = %d, want %d (clamped)", got, want)
	}
}
