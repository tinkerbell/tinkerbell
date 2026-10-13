package bootstrap

import (
	"bytes"
	"math"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestArchiveModTime(t *testing.T) {
	files := []File{{Name: "a/b.txt", Data: []byte("x")}}
	stamp := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	got, err := Archive(files, stamp)
	if err != nil {
		t.Fatal(err)
	}
	// Directories carry the stamp too, so ls -l is consistent for the tree.
	for _, e := range readArchive(t, got) {
		if e.mtime != uint32(stamp.Unix()) {
			t.Errorf("%s mtime = %d, want %d", e.name, e.mtime, stamp.Unix())
		}
	}
}

// TestArchiveModTimeClamping keeps out-of-range times from wrapping into a
// nonsense date, since the newc field is 32 bits unsigned.
func TestArchiveModTimeClamping(t *testing.T) {
	tests := map[string]struct {
		in   time.Time
		want uint32
	}{
		"zero time is the epoch":    {time.Time{}, 0},
		"before the epoch":          {time.Date(1969, 1, 1, 0, 0, 0, 0, time.UTC), 0},
		"epoch itself":              {time.Unix(0, 0), 0},
		"after the 32 bit rollover": {time.Date(2110, 1, 1, 0, 0, 0, 0, time.UTC), math.MaxUint32},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := unixSeconds(tt.in); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// TestArchiveIsStableAcrossCalls is the property the ISO path depends on: one
// BMC mount is assembled from thousands of independently generated responses,
// so repeated generation must produce identical bytes.
func TestArchiveIsStableAcrossCalls(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec: tinkerbell.HardwareSpec{Metadata: &tinkerbell.HardwareMetadata{
			OSIEFiles: []tinkerbell.OSIEFile{{Path: "a.txt", Contents: "hello"}},
		}},
	}

	first, err := Files(hw)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Archive(first, ModTime(hw))
	if err != nil {
		t.Fatal(err)
	}

	for range 50 {
		files, err := Files(hw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Archive(files, ModTime(hw))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("repeated generation produced different bytes; range requests would splice mismatched headers")
		}
	}
}

func TestModTime(t *testing.T) {
	stamp := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		hw   *tinkerbell.Hardware
		want time.Time
	}{
		"nil hardware":    {nil, time.Time{}},
		"unset timestamp": {&tinkerbell.Hardware{}, time.Time{}},
		"creation time":   {&tinkerbell.Hardware{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(stamp)}}, stamp},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ModTime(tt.hw); !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
