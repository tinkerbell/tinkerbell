package iso

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
)

// The slot contract with the CaptainOS build.
//
// An ISO reserves the slot as a file of whole sectors. Every sector of the
// unpersonalized slot begins with a header identifying it and the rest of the
// sector is zero, so the slot is found by content as the ISO streams through,
// like the kernel command line magic string, with no metadata to fetch.
//
// Each header is a newc TRAILER!!! record, an empty archive, whose otherwise
// unused numeric fields carry a fixed magic, the contract version, the
// sector's index and the slot's sector count. An unpersonalized slot is
// therefore a run of empty archives that the kernel extracts as nothing, so the
// ISO still boots.
//
//	c_mtime, c_devmajor, c_devminor: slotMagic
//	c_rdevmajor:                     slotVersion
//	c_rdevminor:                     sector index
//	c_check:                         sector count
//	c_nlink:                         1
//	c_namesize:                      11, for "TRAILER!!!\0"
//	all other fields:                0
//
// Fields are written as 8 upper case hex digits, as the newc format requires
// and CaptainOS writes them.
const (
	// sectorSize is the size of each slot sector, and of the ISO sector.
	sectorSize = 2048
	// slotVersion is the only slot contract version understood here.
	slotVersion = 1
)

var slotMagic = [3]uint32{0x1E2D339C, 0xF7CBD880, 0x9A9E3913}

// sectorHeaderLen is a 110 byte newc header, the 11 byte name and 3 bytes of
// padding.
const sectorHeaderLen = 124

// headerPrefix is the part of every sector header that does not vary: the newc
// magic through c_rdevmajor.
var headerPrefix = newcFields("070701", 0, 0, 0, 0, 1, slotMagic[0], 0, slotMagic[1], slotMagic[2], slotVersion)

const headerSuffix = "TRAILER!!!\x00\x00\x00\x00"

func newcFields(magic string, fields ...uint32) []byte {
	b := []byte(magic)
	for _, f := range fields {
		b = fmt.Appendf(b, "%08X", f)
	}
	return b
}

// sectorHeader returns the header of sector index of a count sector slot.
func sectorHeader(index, count uint32) []byte {
	h := append(bytes.Clone(headerPrefix), newcFields("", index, 11, count)...)
	return append(h, headerSuffix...)
}

// parseSectorHeader returns the sector index and slot sector count of h, which
// begins with headerPrefix and is sectorHeaderLen bytes.
func parseSectorHeader(h []byte) (index, count int64, ok bool) {
	rest := h[len(headerPrefix):]
	idx, err := strconv.ParseUint(string(rest[0:8]), 16, 32)
	if err != nil || string(rest[8:16]) != "0000000B" {
		return 0, 0, false
	}
	n, err := strconv.ParseUint(string(rest[16:24]), 16, 32)
	if err != nil || string(rest[24:]) != headerSuffix || n == 0 || idx >= n {
		return 0, 0, false
	}
	return int64(idx), int64(n), true
}

// slotPlaceholder returns an unpersonalized slot of sectors sectors, as the
// CaptainOS build reserves it.
func slotPlaceholder(sectors int) []byte {
	b := make([]byte, sectors*sectorSize)
	for i := range sectors {
		copy(b[i*sectorSize:], sectorHeader(uint32(i), uint32(sectors))) //nolint:gosec // sectors is small.
	}
	return b
}

// slot writes a Hardware's archive into the bootstrap slot of one ISO response
// as it streams through.
//
// It is best effort, like the kernel command line patch: a response in which
// no sector header is found is passed through unmodified. A response that
// starts partway into a sector is placed by the next header, so up to a sector
// is held back until a header is found or the response is complete. A response
// shorter than that, starting and ending inside a single sector of the
// archive, cannot be placed and is passed through unmodified.
//
// Only the sectors the archive occupies are rewritten. The rest keep their
// headers, which are empty archives, so they read the same whether or not a
// response could be placed.
type slot struct {
	// archive returns the archive to write. It is called once, when a sector
	// header is first found.
	archive func() ([]byte, error)
	// err reports why a slot that was found was not personalized.
	err error

	pending []byte // received bytes not yet returned
	sent    int    // length of the pending prefix the last call returned
	pos     int64  // stream offset of pending[0]
	scan    int    // where in pending the header search resumes
	found   bool
	overlay *overlay
}

// holdBack is how many trailing bytes cannot be sent before the slot is found:
// a sector, before a header that may not have been fully received.
const holdBack = sectorSize + sectorHeaderLen - 1

// next takes the next bytes of the stream and returns those ready to send. The
// result is valid until the next call; b is not retained.
func (s *slot) next(b []byte) []byte {
	s.drop()
	s.pending = append(s.pending, b...)
	if !s.found {
		s.find()
	}
	ready := len(s.pending)
	if s.overlay != nil {
		s.overlay.apply(s.pending, s.pos)
	} else if !s.found {
		ready = max(0, ready-holdBack)
	}
	s.sent = ready
	return s.pending[:ready]
}

// flush returns the bytes still held back once the stream has ended.
func (s *slot) flush() []byte {
	s.drop()
	s.sent = len(s.pending)
	return s.pending
}

// drop discards the bytes the last call returned.
func (s *slot) drop() {
	n := copy(s.pending, s.pending[s.sent:])
	s.pending = s.pending[:n]
	s.pos += int64(s.sent)
	s.scan = max(0, s.scan-s.sent)
	s.sent = 0
}

func (s *slot) find() {
	for {
		i := bytes.Index(s.pending[s.scan:], headerPrefix)
		if i < 0 {
			// A header may begin in the last bytes, once more arrive.
			s.scan = max(s.scan, len(s.pending)-len(headerPrefix)+1)
			return
		}
		h := s.scan + i
		if len(s.pending)-h < sectorHeaderLen {
			s.scan = h
			return
		}
		index, count, ok := parseSectorHeader(s.pending[h : h+sectorHeaderLen])
		if !ok {
			s.scan = h + 1
			continue
		}
		s.found = true
		s.place(s.pos+int64(h)-index*sectorSize, count)
		return
	}
}

// place prepares the overlay of the slot that starts at stream offset start.
func (s *slot) place(start, sectors int64) {
	if s.archive == nil {
		s.err = errors.New("no archive to write into the slot")
		return
	}
	a, err := s.archive()
	if err != nil {
		s.err = err
		return
	}
	// Truncating would produce an archive the kernel cannot parse.
	if capacity := sectors * sectorSize; int64(len(a)) > capacity {
		s.err = fmt.Errorf("bootstrap archive is %d bytes, slot capacity is %d", len(a), capacity)
		return
	}
	span := (int64(len(a)) + sectorSize - 1) / sectorSize * sectorSize
	s.overlay = &overlay{offset: start, capacity: span, archive: a}
}

// overlay replaces [offset, offset+len(archive)) of a stream with archive and
// zeroes [offset+len(archive), offset+capacity).
type overlay struct {
	offset, capacity int64
	archive          []byte
}

// apply rewrites the portion of buf that intersects the overlay. start is the
// stream offset of buf[0].
func (o *overlay) apply(buf []byte, start int64) {
	lo := max(start, o.offset)
	hi := min(start+int64(len(buf)), o.offset+o.capacity)
	if lo >= hi {
		return
	}
	dst := buf[lo-start : hi-start]
	if rel := lo - o.offset; rel < int64(len(o.archive)) {
		n := min(int64(len(dst)), int64(len(o.archive))-rel)
		copy(dst[:n], o.archive[rel:rel+n])
		dst = dst[n:]
	}
	// The rest of the archive's last sector is zero padding, which the kernel
	// skips when walking the initramfs archive sequence.
	clear(dst)
}

type slotCtxKey struct{}

// withSlot returns ctx carrying s.
func withSlot(ctx context.Context, s *slot) context.Context {
	return context.WithValue(ctx, slotCtxKey{}, s)
}

// slotFrom returns the slot stored in ctx, or nil.
func slotFrom(ctx context.Context) *slot {
	s, _ := ctx.Value(slotCtxKey{}).(*slot)
	return s
}
