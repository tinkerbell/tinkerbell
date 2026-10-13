package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

const testMAC = "de:ed:be:ef:fe:ed"

type notFoundError struct{}

func (notFoundError) Error() string  { return "hardware not found" }
func (notFoundError) NotFound() bool { return true }

type stubBackend struct {
	hw  *tinkerbell.Hardware
	err error
}

func (s *stubBackend) FilterHardware(_ context.Context, _ data.HardwareFilter) (*tinkerbell.Hardware, error) {
	return s.hw, s.err
}

// hardwareWithFiles builds a netboot-enabled Hardware for testMAC carrying the given files.
func hardwareWithFiles(files ...tinkerbell.OSIEFile) *tinkerbell.Hardware {
	allow := true
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Interfaces: []tinkerbell.Interface{{
				DHCP:    &tinkerbell.DHCP{MAC: testMAC},
				Netboot: &tinkerbell.Netboot{AllowPXE: &allow},
			}},
			Metadata: &tinkerbell.HardwareMetadata{OSIEFiles: files},
		},
	}
}

func serve(t *testing.T, b BackendReader, method, path string) *http.Response {
	t.Helper()
	h := &Handler{Logger: logr.Discard(), Backend: b}
	w := httptest.NewRecorder()
	h.HandlerFunc()(w, httptest.NewRequest(method, path, nil))
	return w.Result()
}

func body(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHandlerServesUserDefinedFiles(t *testing.T) {
	hw := hardwareWithFiles(
		tinkerbell.OSIEFile{Path: "network.json", Contents: `{"version":1}`},
		tinkerbell.OSIEFile{Path: "/etc/motd", Contents: "welcome\n"},
		tinkerbell.OSIEFile{Path: "scripts/setup.sh", Contents: "#!/bin/sh\necho hi\n"},
	)
	resp := serve(t, &stubBackend{hw: hw}, http.MethodGet, "/bootstrap/"+testMAC+"/"+ArchiveName)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	b := body(t, resp)
	if got := resp.Header.Get("Content-Length"); got != strconv.Itoa(len(b)) {
		t.Errorf("Content-Length %q does not match the %d byte body", got, len(b))
	}

	entries := readArchive(t, b)
	var got []string
	for _, e := range entries {
		got = append(got, e.name)
	}
	want := []string{
		Dir,
		Dir + "/etc",
		Dir + "/etc/motd",
		Dir + "/network.json",
		Dir + "/scripts",
		Dir + "/scripts/setup.sh",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("archive contents:\n got %v\nwant %v", got, want)
	}

	for _, e := range entries {
		switch e.name {
		case Dir + "/etc/motd":
			if string(e.data) != "welcome\n" {
				t.Errorf("motd = %q", e.data)
			}
		case Dir + "/scripts/setup.sh":
			if e.mode != typeFile|DefaultMode {
				t.Errorf("setup.sh mode = %o, want %o", e.mode, typeFile|DefaultMode)
			}
		}
	}
}

// TestHandlerContentIsVerbatim guards against the payloads being reformatted or
// re-encoded on the way through.
func TestHandlerContentIsVerbatim(t *testing.T) {
	const raw = "#!/bin/sh\necho 'hello'  \t\n\n# trailing whitespace preserved   \n"
	hw := hardwareWithFiles(tinkerbell.OSIEFile{Path: "setup.sh", Contents: raw})
	resp := serve(t, &stubBackend{hw: hw}, http.MethodGet, "/bootstrap/"+testMAC+"/"+ArchiveName)
	defer resp.Body.Close()

	for _, e := range readArchive(t, body(t, resp)) {
		if e.name == Dir+"/setup.sh" {
			if string(e.data) != raw {
				t.Fatalf("content altered:\ngot  %q\nwant %q", e.data, raw)
			}
			return
		}
	}
	t.Fatal("file was not delivered")
}

// TestHandlerNoFiles covers a Hardware that defines none: the response is a
// valid, empty archive rather than an error.
func TestHandlerNoFiles(t *testing.T) {
	for name, hw := range map[string]*tinkerbell.Hardware{
		"no metadata": func() *tinkerbell.Hardware { h := hardwareWithFiles(); h.Spec.Metadata = nil; return h }(),
		"no files":    hardwareWithFiles(),
	} {
		t.Run(name, func(t *testing.T) {
			resp := serve(t, &stubBackend{hw: hw}, http.MethodGet, "/bootstrap/"+testMAC+"/"+ArchiveName)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
			}
			b := body(t, resp)
			if entries := readArchive(t, b); len(entries) != 0 {
				t.Fatalf("expected an empty archive, got %v", entries)
			}
			if !strings.Contains(string(b), trailerName) {
				t.Error("empty archive is missing its trailer")
			}
		})
	}
}

func TestHandlerHEADMatchesGET(t *testing.T) {
	b := &stubBackend{hw: hardwareWithFiles(tinkerbell.OSIEFile{Path: "a.txt", Contents: "a"})}
	path := "/bootstrap/" + testMAC + "/" + ArchiveName

	get := serve(t, b, http.MethodGet, path)
	defer get.Body.Close()
	head := serve(t, b, http.MethodHead, path)
	defer head.Body.Close()

	if get.Header.Get("Content-Length") != head.Header.Get("Content-Length") {
		t.Errorf("HEAD Content-Length %q does not match GET %q",
			head.Header.Get("Content-Length"), get.Header.Get("Content-Length"))
	}
	buf := make([]byte, 1)
	if n, _ := head.Body.Read(buf); n > 0 {
		t.Error("HEAD response carried a body")
	}
}

func TestHandlerErrors(t *testing.T) {
	notFound := fmt.Errorf("filter hardware: %w", notFoundError{})
	good := hardwareWithFiles(tinkerbell.OSIEFile{Path: "a.txt", Contents: "a"})
	denied := hardwareWithFiles(tinkerbell.OSIEFile{Path: "a.txt", Contents: "a"})
	*denied.Spec.Interfaces[0].Netboot.AllowPXE = false

	tests := map[string]struct {
		method  string
		path    string
		backend BackendReader
		want    int
	}{
		"POST is not allowed": {
			method:  http.MethodPost,
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{hw: good},
			want:    http.StatusMethodNotAllowed,
		},
		"allowPXE false": {
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{hw: denied},
			want:    http.StatusForbidden,
		},
		"no matching interface": {
			path:    "/bootstrap/00:00:00:00:00:01/" + ArchiveName,
			backend: &stubBackend{hw: good},
			want:    http.StatusForbidden,
		},
		"unknown archive name": {
			path:    "/bootstrap/" + testMAC + "/other.cpio",
			backend: &stubBackend{hw: good},
			want:    http.StatusNotFound,
		},
		"invalid mac": {
			path:    "/bootstrap/not-a-mac/" + ArchiveName,
			backend: &stubBackend{},
			want:    http.StatusBadRequest,
		},
		"no hardware record": {
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{err: notFound},
			want:    http.StatusNotFound,
		},
		"backend failure": {
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{err: errors.New("boom")},
			want:    http.StatusInternalServerError,
		},
		"no backend": {
			path: "/bootstrap/" + testMAC + "/" + ArchiveName,
			want: http.StatusInternalServerError,
		},
		"empty path": {
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{hw: hardwareWithFiles(tinkerbell.OSIEFile{Path: "  ", Contents: "x"})},
			want:    http.StatusInternalServerError,
		},
		"parent directory traversal": {
			path:    "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{hw: hardwareWithFiles(tinkerbell.OSIEFile{Path: "../etc/shadow", Contents: "x"})},
			want:    http.StatusInternalServerError,
		},
		"duplicate paths": {
			path: "/bootstrap/" + testMAC + "/" + ArchiveName,
			backend: &stubBackend{hw: hardwareWithFiles(
				tinkerbell.OSIEFile{Path: "a.txt", Contents: "one"},
				tinkerbell.OSIEFile{Path: "/a.txt", Contents: "two"},
			)},
			want: http.StatusInternalServerError,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			resp := serve(t, tt.backend, method, tt.path)
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("got status %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}
