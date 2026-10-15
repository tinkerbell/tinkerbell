package bootstrap

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// entry is a parsed newc CPIO record.
type entry struct {
	name  string
	mode  uint32
	mtime uint32
	data  []byte
}

// readArchive is a minimal newc reader used to prove the generated archives are
// parseable the same way the kernel parses them.
func readArchive(t *testing.T, b []byte) []entry {
	t.Helper()
	var out []entry
	pos := 0
	field := func(i int) uint32 {
		v, err := strconv.ParseUint(string(b[pos+6+i*8:pos+6+(i+1)*8]), 16, 32)
		if err != nil {
			t.Fatalf("field %d at %d: %v", i, pos, err)
		}
		return uint32(v)
	}
	align4 := func(n int) int { return (n + 3) &^ 3 }
	for {
		if pos+headerLen > len(b) {
			t.Fatalf("archive ended without a trailer")
		}
		if string(b[pos:pos+6]) != newcMagic {
			t.Fatalf("bad magic at offset %d: %q", pos, b[pos:pos+6])
		}
		mode := field(1)
		mtime := field(5)
		size := int(field(6))
		nameSize := int(field(11))
		nameStart := pos + headerLen
		name := string(b[nameStart : nameStart+nameSize-1])
		dataStart := align4(nameStart + nameSize)
		if name == trailerName {
			return out
		}
		out = append(out, entry{name: name, mode: mode, mtime: mtime, data: b[dataStart : dataStart+size]})
		pos = align4(dataStart + size)
	}
}

func TestArchive(t *testing.T) {
	tests := map[string]struct {
		files []File
		want  []entry
	}{
		"empty": {
			files: nil,
			want:  nil,
		},
		"single file with parent directories": {
			files: []File{{Name: "tinkerbell/bootstrap/network.json", Data: []byte(`{"a":1}`)}},
			want: []entry{
				{name: "tinkerbell", mode: typeDir | 0o755, data: []byte{}},
				{name: "tinkerbell/bootstrap", mode: typeDir | 0o755, data: []byte{}},
				{name: "tinkerbell/bootstrap/network.json", mode: typeFile | DefaultMode, data: []byte(`{"a":1}`)},
			},
		},
		"binary payload and shared directories are emitted once": {
			files: []File{
				{Name: "/tinkerbell/bootstrap/b.bin", Data: []byte{0x00, 0xff, 0x00, 0x1b, 0x5b, 0x41}},
				{Name: "tinkerbell/bootstrap/a.txt", Data: []byte("hello"), Mode: 0o600},
			},
			want: []entry{
				{name: "tinkerbell", mode: typeDir | 0o755, data: []byte{}},
				{name: "tinkerbell/bootstrap", mode: typeDir | 0o755, data: []byte{}},
				{name: "tinkerbell/bootstrap/a.txt", mode: typeFile | 0o600, data: []byte("hello")},
				{name: "tinkerbell/bootstrap/b.bin", mode: typeFile | DefaultMode, data: []byte{0x00, 0xff, 0x00, 0x1b, 0x5b, 0x41}},
			},
		},
		"zero length file": {
			files: []File{{Name: "empty", Data: nil}},
			want:  []entry{{name: "empty", mode: typeFile | DefaultMode, data: []byte{}}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Archive(tt.files, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got)%4 != 0 {
				t.Errorf("archive length %d is not 4 byte aligned", len(got))
			}
			entries := readArchive(t, got)
			if diff := cmp.Diff(tt.want, entries, cmp.AllowUnexported(entry{})); diff != "" {
				t.Errorf("unexpected archive contents (-want +got):\n%s", diff)
			}
		})
	}
}

func TestArchiveErrors(t *testing.T) {
	tests := map[string][]File{
		"empty name":                          {{Name: ""}},
		"root name":                           {{Name: "/"}},
		"escaping name":                       {{Name: "../etc/shadow"}},
		"dot segment":                         {{Name: "osie_files/."}},
		"NUL in name":                         {{Name: "a\x00b"}},
		"duplicate name":                      {{Name: "a/b"}, {Name: "/a/b"}},
		"file as parent":                      {{Name: "net"}, {Name: "net/config.json"}},
		"file as parent, other names between": {{Name: "a"}, {Name: "a-x"}, {Name: "a/b/c"}},
	}
	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Archive(files, time.Time{}); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestArchiveIsDeterministic(t *testing.T) {
	files := []File{
		{Name: "tinkerbell/bootstrap/network.json", Data: []byte("{}")},
		{Name: "tinkerbell/bootstrap/extra", Data: []byte("x")},
	}
	first, err := Archive(files, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// Reversed input must produce the same bytes.
	second, err := Archive([]File{files[1], files[0]}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("archive generation is not deterministic")
	}
}

func BenchmarkArchive(b *testing.B) {
	files := make([]File, 16)
	for i := range files {
		files[i] = File{Name: fmt.Sprintf("tinkerbell/bootstrap/f%d.json", i), Data: bytes.Repeat([]byte("x"), 1024)}
	}
	for b.Loop() {
		if _, err := Archive(files, time.Time{}); err != nil {
			b.Fatal(err)
		}
	}
}
