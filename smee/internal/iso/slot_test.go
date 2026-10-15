package iso

import (
	"bytes"
	"errors"
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// stream feeds data through s in chunks of random size up to maxChunk, and
// returns the output.
func stream(s *slot, data []byte, rnd *rand.Rand, maxChunk int) []byte {
	var out []byte
	for off := 0; off < len(data); {
		n := min(rnd.Intn(maxChunk)+1, len(data)-off)
		chunk := bytes.Clone(data[off : off+n])
		out = append(out, s.next(chunk)...)
		clear(chunk) // the slot must not retain its input
		off += n
	}
	return append(out, s.flush()...)
}

func archiveSlot(archive []byte) *slot {
	return &slot{archive: func() ([]byte, error) { return archive, nil }}
}

// TestSlotPlaceholderIsEmptyArchives checks every sector against the newc
// format independently of the encoder, since an unpersonalized slot must boot.
func TestSlotPlaceholderIsEmptyArchives(t *testing.T) {
	p := slotPlaceholder(testSlotSectors)
	if len(p) != testSlotSectors*sectorSize {
		t.Fatalf("got %d bytes, want %d", len(p), testSlotSectors*sectorSize)
	}
	field := func(h []byte, n int) uint64 {
		v, err := strconv.ParseUint(string(h[6+8*n:6+8*n+8]), 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for i := range testSlotSectors {
		sector := p[i*sectorSize : (i+1)*sectorSize]
		switch {
		case string(sector[:6]) != "070701":
			t.Fatalf("sector %d does not start with the newc magic", i)
		case field(sector, 1) != 0 || field(sector, 6) != 0:
			t.Fatalf("sector %d header has a mode or file data", i)
		case field(sector, 11) != 11 || string(sector[110:121]) != "TRAILER!!!\x00":
			t.Fatalf("sector %d header is not a TRAILER!!! record", i)
		case field(sector, 10) != uint64(i) || field(sector, 12) != testSlotSectors:
			t.Fatalf("sector %d header does not carry its index and the sector count", i)
		case !bytes.Equal(sector[121:], make([]byte, sectorSize-121)):
			t.Fatalf("sector %d is not zero after its header", i)
		}
	}
}

// FuzzSlot checks the rule for any response: one holding a whole sector
// header is personalized, and any other is passed through, however the
// stream is split into reads. `go test` runs the seeds below; `go test
// -fuzz=FuzzSlot` explores further.
func FuzzSlot(f *testing.F) {
	src := sourceISO(testSlotOff)
	archive := testArchive(f)
	want := personalize(src, archive)

	// start, length (0 for the rest of the image) and read size seed.
	f.Add(0, 0, 1)
	f.Add(0, 0, 7)
	f.Add(0, 0, sectorSize)
	f.Add(0, 0, 32*1024)
	f.Add(0, testSlotOff, 4096)                               // entirely before the slot
	f.Add(testSlotOff+10, 2*sectorSize+sectorHeaderLen, 4096) // starting partway into a sector
	f.Add(testSlotOff+sectorSize+200, 1000, 4096)             // inside one sector, without a header
	f.Add(testSlotOff+len(archive)-10, testSlotEnd, 4096)     // crossing the archive end
	f.Add(testSlotEnd-50, 100, 4096)                          // crossing the slot end
	f.Add(testSlotEnd, 0, 4096)                               // entirely after the slot
	f.Add(1, math.MaxInt, 1)                                  // a length that overflows start+length
	for k := -2; k <= testSlotSectors+1; k++ {
		for sectors := 1; sectors <= 3; sectors++ {
			f.Add(testSlotOff+k*sectorSize, sectors*sectorSize, 8192) // BMC sector reads
		}
	}
	for i := range testSlotSectors {
		h := testSlotOff + i*sectorSize
		for _, start := range []int{h - 1, h, h + 1, h + sectorHeaderLen} {
			for _, length := range []int{sectorHeaderLen, sectorSize, holdBack, 2 * sectorSize} {
				f.Add(start, length, 333)
			}
		}
	}

	f.Fuzz(func(t *testing.T, start, length, chunk int) {
		if start < 0 || length < 0 || chunk < 1 || start >= len(src) {
			t.Skip()
		}
		end := len(src)
		// Compared before adding, so a huge length cannot overflow.
		if length > 0 && length < end-start {
			end = start + length
		}
		got := stream(archiveSlot(archive), src[start:end], rand.New(rand.NewSource(int64(chunk))), chunk)
		expect := want[start:end]
		if !holdsHeader(start, end) {
			expect = src[start:end]
		}
		if !bytes.Equal(got, expect) {
			t.Fatalf("range [%d,%d) in reads of up to %d bytes produced wrong bytes", start, end, chunk)
		}
	})
}

func TestSlotNotFound(t *testing.T) {
	src := sourceISO(-1)
	// Bytes that begin like a header must not be taken for one.
	copy(src[sectorSize:], headerPrefix)
	called := false
	s := &slot{archive: func() ([]byte, error) { called = true; return nil, nil }}
	if got := stream(s, src, rand.New(rand.NewSource(1)), 1000); !bytes.Equal(got, src) {
		t.Fatal("a stream without a slot was modified")
	}
	if called {
		t.Fatal("the archive was built for a stream without a slot")
	}
}

func TestSlotNotPersonalized(t *testing.T) {
	for name, archive := range map[string]func() ([]byte, error){
		"archive over capacity": func() ([]byte, error) { return make([]byte, testSlotSectors*sectorSize+1), nil },
		"archive error":         func() ([]byte, error) { return nil, errors.New("boom") },
		"no archive":            nil,
	} {
		t.Run(name, func(t *testing.T) {
			src := sourceISO(testSlotOff)
			s := &slot{archive: archive}
			if got := stream(s, src, rand.New(rand.NewSource(2)), 5000); !bytes.Equal(got, src) {
				t.Fatal("a slot that could not be personalized was modified")
			}
			if s.err == nil {
				t.Fatal("expected the reason the slot was not personalized")
			}
		})
	}
}

// TestSlotArchiveSizes covers archives ending where the rest of their last
// sector still holds part of its header, at a sector boundary, and at the end
// of the slot.
func TestSlotArchiveSizes(t *testing.T) {
	for name, size := range map[string]int{
		"ending inside a sector header": sectorSize + 50,
		"exactly one sector":            sectorSize,
		"filling the slot":              testSlotSectors * sectorSize,
	} {
		t.Run(name, func(t *testing.T) {
			archive := bytes.Repeat([]byte{0xAB}, size)
			src := sourceISO(testSlotOff)
			if got := stream(archiveSlot(archive), src, rand.New(rand.NewSource(3)), 999); !bytes.Equal(got, personalize(src, archive)) {
				t.Fatal("the archive was not written, zero padded to the end of its last sector")
			}
		})
	}
}

// TestSlotSkipsInvalidHeaders covers bytes that begin like a sector header but
// are not one, before the real slot.
func TestSlotSkipsInvalidHeaders(t *testing.T) {
	valid := sectorHeader(0, 1)
	p := len(headerPrefix)
	for name, decoy := range map[string][]byte{
		"index past count": sectorHeader(3, 3),
		"zero count":       sectorHeader(0, 0),
		"bad name size":    append(append(bytes.Clone(valid[:p+8]), "0000000C"...), valid[p+16:]...),
		"bad name":         append(bytes.Clone(valid[:sectorHeaderLen-14]), "TRAILER!!?\x00\x00\x00\x00"...),
		"not hex":          append(append(bytes.Clone(valid[:p]), "0000000G"...), valid[p+8:]...),
	} {
		t.Run(name, func(t *testing.T) {
			if len(decoy) != sectorHeaderLen {
				t.Fatalf("decoy is %d bytes, want %d", len(decoy), sectorHeaderLen)
			}
			archive := testArchive(t)
			src := sourceISO(testSlotOff)
			copy(src[100:], decoy)
			want := personalize(src, archive)
			if got := stream(archiveSlot(archive), src, rand.New(rand.NewSource(4)), 700); !bytes.Equal(got, want) {
				t.Fatal("a decoy header changed how the slot was placed")
			}
		})
	}
}
