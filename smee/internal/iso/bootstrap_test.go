package iso

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/smee/internal/bootstrap"
)

const (
	testISOSize     = 64 * 1024
	testSlotOff     = 16 * 1024 // sector aligned, as files are in an ISO 9660 image
	testSlotSectors = 4
	testSlotEnd     = testSlotOff + testSlotSectors*sectorSize
	testMAC         = "de:ed:be:ef:fe:ed"
	testISOName     = "captainos.iso"
	unusedMagic     = "this-magic-string-is-not-in-the-test-iso"
	testISOPath     = "/iso/" + testMAC + "/" + testISOName
	sourceISOURL    = "/" + testISOName
)

// sourceISO returns a synthetic ISO with a bootstrap slot at slotOff, or none
// if slotOff is negative.
func sourceISO(slotOff int) []byte {
	src := make([]byte, testISOSize)
	for i := range src {
		src[i] = byte(i%251) + 1 // never zero, so slot padding is distinguishable
	}
	if slotOff >= 0 {
		copy(src[slotOff:], slotPlaceholder(testSlotSectors))
	}
	return src
}

// personalize returns src, with its slot at testSlotOff, with archive written
// into the slot, as Smee serves it.
func personalize(src, archive []byte) []byte {
	want := bytes.Clone(src)
	copy(want[testSlotOff:], archive)
	span := (len(archive) + sectorSize - 1) / sectorSize * sectorSize
	clear(want[testSlotOff+len(archive) : testSlotOff+span])
	return want
}

// holdsHeader reports whether [start, end) of sourceISO(testSlotOff) holds a
// whole sector header, which is what places a response in the slot.
func holdsHeader(start, end int) bool {
	for i := range testSlotSectors {
		h := testSlotOff + i*sectorSize
		if h >= start && h+sectorHeaderLen <= end {
			return true
		}
	}
	return false
}

// testBootstrapFiles make an archive spanning several, but not all, sectors.
var testBootstrapFiles = []tinkerbell.OSIEFile{
	{Path: "network.json", Contents: `{"version":1,"hostname":"test-machine"}`},
	{Path: "/etc/motd", Contents: strings.Repeat("welcome\n", 600)},
}

// testCreationTime stands in for the Hardware's creation timestamp, which is
// what the delivered files are stamped with.
var testCreationTime = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

// testArchive returns the archive built from testBootstrapFiles.
func testArchive(tb testing.TB) []byte {
	tb.Helper()
	files := make([]bootstrap.File, 0, len(testBootstrapFiles))
	for _, f := range testBootstrapFiles {
		files = append(files, bootstrap.File{
			Name: bootstrap.Dir + "/" + strings.TrimPrefix(f.Path, "/"),
			Data: []byte(f.Contents),
		})
	}
	a, err := bootstrap.Archive(files, testCreationTime)
	if err != nil {
		tb.Fatal(err)
	}
	if len(a) <= sectorSize || len(a) >= testSlotSectors*sectorSize {
		tb.Fatalf("test archive is %d bytes; it must span several, but not all, slot sectors", len(a))
	}
	return a
}

// bootstrapBackend serves a Hardware with files, or testBootstrapFiles if nil.
type bootstrapBackend struct {
	files []tinkerbell.OSIEFile
}

func (b bootstrapBackend) FilterHardware(_ context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error) {
	if opts.ByMACAddress == "" {
		return nil, fmt.Errorf("hardware not found")
	}
	files := b.files
	if files == nil {
		files = testBootstrapFiles
	}
	return &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(testCreationTime)},
		Spec: tinkerbell.HardwareSpec{
			Interfaces: []tinkerbell.Interface{{
				DHCP:    &tinkerbell.DHCP{MAC: opts.ByMACAddress, VLANID: "400"},
				Netboot: &tinkerbell.Netboot{},
			}},
			Metadata: &tinkerbell.HardwareMetadata{
				Facility:  &tinkerbell.MetadataFacility{FacilityCode: "test"},
				OSIEFiles: files,
			},
		},
	}, nil
}

// serveSourceISO serves src with range support and returns its URL.
func serveSourceISO(t *testing.T, src []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testISOName), src, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	return srv.URL + sourceISOURL
}

func newISOHandler(t *testing.T, srcURL string, bootstrapEnabled bool, backend BackendReader) http.HandlerFunc {
	t.Helper()
	h := &Handler{
		Logger:           logr.Discard(),
		Backend:          backend,
		Patch:            Patch{MagicString: unusedMagic, SourceISO: srcURL},
		BootstrapEnabled: bootstrapEnabled,
	}
	hf, err := h.HandlerFunc()
	if err != nil {
		t.Fatal(err)
	}
	return hf
}

// serveISO requests the ISO with the given header key and value pairs.
func serveISO(t *testing.T, hf http.HandlerFunc, header ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, testISOPath, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	hf.ServeHTTP(w, req)
	return w.Result()
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fetchISO serves the ISO and returns its body.
func fetchISO(t *testing.T, hf http.HandlerFunc, header ...string) []byte {
	t.Helper()
	resp := serveISO(t, hf, header...)
	defer resp.Body.Close()
	return readBody(t, resp)
}

// TestBootstrapSlot covers whether a full ISO download is personalized.
func TestBootstrapSlot(t *testing.T) {
	tests := map[string]struct {
		noSlot       bool
		disabled     bool
		files        []tinkerbell.OSIEFile
		upstream     func(t *testing.T, src []byte) string
		header       []string
		personalized bool
	}{
		"personalized": {personalized: true},
		// For example to object storage; the slot is found in whatever
		// response is finally served.
		"redirected": {upstream: redirectingUpstream, personalized: true},
		// The slot is only found in unencoded bytes, so Smee must ask for them.
		"compressing upstream": {upstream: compressingUpstream, header: []string{"Accept-Encoding", "gzip"}, personalized: true},
		// Such as HookOS.
		"no slot":  {noSlot: true},
		"disabled": {disabled: true},
		// Truncating would leave an archive the kernel cannot parse.
		"archive over capacity": {files: []tinkerbell.OSIEFile{{Path: "big", Contents: strings.Repeat("x", testSlotSectors*sectorSize)}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			slotOff := testSlotOff
			if tt.noSlot {
				slotOff = -1
			}
			src := sourceISO(slotOff)
			upstream := tt.upstream
			if upstream == nil {
				upstream = serveSourceISO
			}
			hf := newISOHandler(t, upstream(t, src), !tt.disabled, bootstrapBackend{files: tt.files})

			resp := serveISO(t, hf, tt.header...)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
			}
			want := src
			if tt.personalized {
				want = personalize(src, testArchive(t))
			}
			if got := readBody(t, resp); !bytes.Equal(got, want) {
				t.Fatalf("got %d bytes with Content-Encoding %q; personalized %v does not match", len(got), resp.Header.Get("Content-Encoding"), tt.personalized)
			}
		})
	}
}

func redirectingUpstream(t *testing.T, src []byte) string {
	t.Helper()
	srcURL := serveSourceISO(t, src)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/relative.iso" {
			http.Redirect(w, r, "/absolute.iso", http.StatusFound)
			return
		}
		http.Redirect(w, r, srcURL+"?sig=abc", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/relative.iso"
}

// compressingUpstream compresses when the client accepts it.
func compressingUpstream(t *testing.T, src []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			_, _ = gz.Write(src)
			return
		}
		_, _ = w.Write(src)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + sourceISOURL
}

// TestBootstrapSlotRanges covers range responses. Where the slot is placed
// within them is covered by FuzzSlot.
func TestBootstrapSlotRanges(t *testing.T) {
	src := sourceISO(testSlotOff)
	hf := newISOHandler(t, serveSourceISO(t, src), true, bootstrapBackend{})
	want := personalize(src, testArchive(t))

	for name, r := range map[string]struct{ start, end int }{
		"holding a header":                    {testSlotOff - 100, testSlotOff + 300},
		"inside one sector, without a header": {testSlotOff + sectorSize + 200, testSlotOff + sectorSize + 1200},
	} {
		t.Run(name, func(t *testing.T) {
			resp := serveISO(t, hf, "Range", fmt.Sprintf("bytes=%d-%d", r.start, r.end-1))
			if resp.StatusCode != http.StatusPartialContent {
				t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusPartialContent)
			}
			expect := want[r.start:r.end]
			if !holdsHeader(r.start, r.end) {
				expect = src[r.start:r.end] // best effort: served as is
			}
			if !bytes.Equal(readBody(t, resp), expect) {
				t.Fatal("range produced wrong bytes")
			}
		})
	}

	// As a BMC reads virtual media: independent single sector reads, in any
	// order, that must add up to one coherent image.
	t.Run("sector reads", func(t *testing.T) {
		got := make([]byte, testISOSize)
		for start := testISOSize - sectorSize; start >= 0; start -= sectorSize {
			copy(got[start:], fetchISO(t, hf, "Range", fmt.Sprintf("bytes=%d-%d", start, start+sectorSize-1)))
		}
		if !bytes.Equal(got, want) {
			t.Fatal("image reassembled from sector reads does not match the full response")
		}
	})
}

// TestBootstrapSlotMultiRange covers a multipart range response, whose parts
// are separated by boundaries the slot must not be written over.
func TestBootstrapSlotMultiRange(t *testing.T) {
	src := sourceISO(testSlotOff)
	hf := newISOHandler(t, serveSourceISO(t, src), true, bootstrapBackend{})

	resp := serveISO(t, hf, "Range", fmt.Sprintf("bytes=0-99,%d-%d", testSlotOff, testSlotEnd-1))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(resp.Body, params["boundary"])
	for _, want := range [][]byte{src[:100], src[testSlotOff:testSlotEnd]} {
		p, err := mr.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(p)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("a multipart part was modified")
		}
	}
}

// TestBootstrapSlotWithKernelParams covers an ISO carrying both the kernel
// command line magic string and the slot, in the same read buffer.
func TestBootstrapSlotWithKernelParams(t *testing.T) {
	// As in a real ISO, long enough to hold the generated kernel parameters.
	magic := strings.Repeat("kernel-params-magic-", 50)
	const magicOff = testSlotOff - 1000
	src := sourceISO(testSlotOff)
	copy(src[magicOff:], magic)
	h := &Handler{
		Logger:           logr.Discard(),
		Backend:          bootstrapBackend{},
		Patch:            Patch{MagicString: magic, SourceISO: serveSourceISO(t, src)},
		BootstrapEnabled: true,
	}
	hf, err := h.HandlerFunc()
	if err != nil {
		t.Fatal(err)
	}

	got := fetchISO(t, hf)
	want := personalize(src, testArchive(t))
	if bytes.Contains(got, []byte(magic)) {
		t.Fatal("the kernel command line was not patched")
	}
	if !bytes.Equal(got[testSlotOff:], want[testSlotOff:]) {
		t.Fatal("the slot was not personalized")
	}
	if !bytes.Equal(got[:magicOff], src[:magicOff]) || !bytes.Equal(got[magicOff+len(magic):testSlotOff], src[magicOff+len(magic):testSlotOff]) {
		t.Fatal("bytes outside the kernel command line and the slot were modified")
	}
}

// TestBootstrapSlotBodylessHTTP2 covers responses without a body served over
// HTTP/2, which rejects even an empty write to them.
func TestBootstrapSlotBodylessHTTP2(t *testing.T) {
	for _, status := range []int{http.StatusNotModified, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(upstream.Close)

			srv := httptest.NewUnstartedServer(newISOHandler(t, upstream.URL+sourceISOURL, true, bootstrapBackend{}))
			srv.EnableHTTP2 = true
			srv.StartTLS()
			t.Cleanup(srv.Close)

			resp, err := srv.Client().Get(srv.URL + testISOPath)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Fatalf("got HTTP/%d, want HTTP/2", resp.ProtoMajor)
			}
			if _, err := io.ReadAll(resp.Body); err != nil {
				t.Fatalf("reading the %d response: %v", status, err)
			}
			if resp.StatusCode != status {
				t.Fatalf("got status %d, want %d", resp.StatusCode, status)
			}
		})
	}
}

type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// TestBootstrapSlotUpstreamError keeps bytes held back for the slot from
// being sent when the upstream response fails before it is complete.
func TestBootstrapSlotUpstreamError(t *testing.T) {
	body := sourceISO(-1)[:3*sectorSize]
	for name, rerr := range map[string]error{"eof": io.EOF, "unexpected eof": io.ErrUnexpectedEOF} {
		t.Run(name, func(t *testing.T) {
			h := &Handler{Logger: logr.Discard()}
			ctx := withSlot(context.Background(), &slot{archive: func() ([]byte, error) { return nil, nil }})
			var dst bytes.Buffer
			n, err := h.Copy(ctx, &dst, &failingReader{data: bytes.Clone(body), err: rerr}, nil)
			if !bytes.Equal(dst.Bytes(), body[:n]) {
				t.Fatal("copied bytes do not match the source")
			}
			if errors.Is(rerr, io.EOF) {
				if err != nil || n != int64(len(body)) {
					t.Fatalf("got %d bytes and error %v, want all %d bytes", n, err, len(body))
				}
				return
			}
			if !errors.Is(err, rerr) || n >= int64(len(body)) {
				t.Fatalf("got %d of %d bytes and error %v; held back bytes must not be sent on a failed response", n, len(body), err)
			}
		})
	}
}
