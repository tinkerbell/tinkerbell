// Package bootstrap builds a Hardware's OSIE files into an uncompressed `newc`
// CPIO archive, which the kernel extracts as an additional initramfs, and
// serves it over HTTP.
package bootstrap

import (
	"bytes"
	"fmt"
	"math"
	"path"
	"slices"
	"strings"
	"time"
)

// Every constant below is fixed by the `newc` format and enforced by the
// kernel's initramfs parser. Changing any of them breaks extraction.
const (
	// Checked byte for byte by the kernel, which otherwise rejects the archive
	// with "no cpio magic".
	newcMagic = "070701"
	// 6 magic bytes plus 13 eight-digit hex fields. Only sizes a buffer here;
	// the test reader relies on it for real parsing.
	headerLen = 110
	// Terminates the archive. Without it the kernel parses on into any padding
	// that follows.
	trailerName = "TRAILER!!!"

	// POSIX S_IFREG and S_IFDIR. The kernel reads these from the mode field to
	// decide which kind of inode to create; the permission bits are ours.
	typeFile = 0o100000
	typeDir  = 0o040000
)

// DefaultMode is applied to files that do not specify one. Delivered files are
// read-only to their owner because they routinely carry credentials.
const DefaultMode = 0o400

// File is a single regular file to place in the generated archive.
type File struct {
	// Name is the archive-relative path, e.g. "osie_files/network/config.json".
	// A leading "/" is stripped; the kernel extracts relative to the initramfs root.
	Name string
	// Mode holds the permission bits, at most 0o777. Zero means DefaultMode,
	// so a mode of no permissions at all cannot be requested.
	Mode uint32
	Data []byte
}

// Archive serializes files into an uncompressed `newc` CPIO archive.
//
// Parent directory entries are synthesized and deduplicated, entries are
// emitted in a deterministic order, and the archive is terminated with the
// mandatory TRAILER!!! entry.
//
// modTime is stamped on every entry. It must be stable for a given set of
// files: the archive is regenerated per HTTP request, and a BMC assembles one
// ISO mount from thousands of range requests, so a moving clock would splice
// mismatched headers together. A zero time yields the epoch.
func Archive(files []File, modTime time.Time) ([]byte, error) {
	entries := make([]File, 0, len(files))
	for _, f := range files {
		name, err := cleanName(f.Name)
		if err != nil {
			return nil, err
		}
		f.Name = name
		if f.Mode&^0o777 != 0 {
			return nil, fmt.Errorf("invalid mode %#o for bootstrap file %q, must not exceed 0777", f.Mode, name)
		}
		if f.Mode == 0 {
			f.Mode = DefaultMode
		}
		// newc records sizes in 32 bits.
		if len(f.Data) > math.MaxUint32 || len(name) >= math.MaxUint32 {
			return nil, fmt.Errorf("bootstrap file %q is too large for a newc archive", name)
		}
		entries = append(entries, f)
	}
	slices.SortFunc(entries, func(a, b File) int { return strings.Compare(a.Name, b.Name) })
	names := make(map[string]bool, len(entries))
	for i, f := range entries {
		if i > 0 && f.Name == entries[i-1].Name {
			return nil, fmt.Errorf("duplicate file in bootstrap archive: %q", f.Name)
		}
		names[f.Name] = true
	}
	// The kernel would replace a file with the directory of the same name.
	for _, f := range entries {
		for _, dir := range parents(f.Name) {
			if names[dir] {
				return nil, fmt.Errorf("bootstrap file %q is also the parent directory of %q", dir, f.Name)
			}
		}
	}

	mtime := unixSeconds(modTime)
	var buf bytes.Buffer
	ino := uint32(1)
	seen := make(map[string]bool)
	for _, f := range entries {
		for _, dir := range parents(f.Name) {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			writeEntry(&buf, &ino, dir, typeDir|0o755, 2, mtime, nil)
		}
		writeEntry(&buf, &ino, f.Name, typeFile|f.Mode, 1, mtime, f.Data)
	}
	// The trailer carries no inode of its own; GNU cpio writes ino 0.
	zero := uint32(0)
	writeEntry(&buf, &zero, trailerName, 0, 1, mtime, nil)

	return buf.Bytes(), nil
}

// unixSeconds clamps t into the newc mtime field, which is 32 bits unsigned.
func unixSeconds(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	sec := t.Unix()
	if sec < 0 {
		return 0
	}
	if sec > math.MaxUint32 {
		return math.MaxUint32
	}

	return uint32(sec)
}

// Empty returns the archive a freshly built, unpersonalized slot contains.
func Empty() []byte {
	a, _ := Archive(nil, time.Time{})
	return a
}

func cleanName(name string) (string, error) {
	// newc names are NUL-terminated, so an embedded NUL would truncate the name.
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("bootstrap file name must not contain NUL: %q", name)
	}
	n := path.Clean("/" + strings.TrimPrefix(name, "/"))
	n = strings.TrimPrefix(n, "/")
	if n == "" || n == "." {
		return "", fmt.Errorf("invalid bootstrap file name: %q", name)
	}
	// Rejected rather than normalized: a dot segment means the caller's
	// intent does not match what would be extracted.
	if slices.ContainsFunc(strings.Split(name, "/"), func(s string) bool { return s == "." || s == ".." }) {
		return "", fmt.Errorf("bootstrap file name must not contain '.' or '..' segments: %q", name)
	}
	return n, nil
}

func parents(name string) []string {
	var dirs []string
	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
	}
	slices.Reverse(dirs)
	return dirs
}

func writeEntry(buf *bytes.Buffer, ino *uint32, name string, mode, nlink, mtime uint32, data []byte) {
	h := make([]byte, 0, headerLen)
	h = append(h, newcMagic...)
	for _, v := range []uint32{
		*ino,
		mode,
		0, // uid
		0, // gid
		nlink,
		mtime,
		uint32(len(data)),     //nolint:gosec // bounded by Archive
		0,                     // devmajor
		0,                     // devminor
		0,                     // rdevmajor
		0,                     // rdevminor
		uint32(len(name) + 1), //nolint:gosec // bounded by Archive
		0,                     // check, unused by the newc format
	} {
		h = fmt.Appendf(h, "%08X", v)
	}
	buf.Write(h)
	buf.WriteString(name)
	buf.WriteByte(0)
	pad4(buf)
	buf.Write(data)
	pad4(buf)
	*ino++
}

func pad4(buf *bytes.Buffer) {
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}
}
