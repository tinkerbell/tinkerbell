package binary

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
)

// captureWriter is an io.ReaderFrom that buffers what is written to it,
// optionally returning a configured error from ReadFrom.
type captureWriter struct {
	buf bytes.Buffer
	err error
}

func (c *captureWriter) ReadFrom(r io.Reader) (int64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return c.buf.ReadFrom(r)
}

// stubRoute returns canned (handled, err) and remembers whether it was
// called. Used to drive Router tests independent of the real routes.
type stubRoute struct {
	name    string
	handled bool
	err     error
	called  bool
}

func (s *stubRoute) Name() string { return s.name }
func (s *stubRoute) TryServe(_ context.Context, _ Request, _ io.ReaderFrom) (bool, error) {
	s.called = true
	return s.handled, s.err
}

// recordRoute records the Request it was handed and reports handled=true.
type recordRoute struct {
	got Request
}

func (r *recordRoute) Name() string { return "record" }
func (r *recordRoute) TryServe(_ context.Context, req Request, _ io.ReaderFrom) (bool, error) {
	r.got = req
	return true, nil
}

// ---------- HandleRead ----------

func TestHandleReadStripsTraceparent(t *testing.T) {
	const want = "pxelinux.cfg/01-08-00-27-00-00-02"
	// A traceparent (version-traceid-spanid-flags) appended to the requested file.
	withTP := want + "-00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"

	rec := &recordRoute{}
	h := TFTP{Log: logr.Discard(), Router: Router{Log: logr.Discard(), Routes: []Route{rec}}}

	if err := h.HandleRead(withTP, &captureWriter{}); err != nil {
		t.Fatal(err)
	}
	if rec.got.Filename != want {
		t.Errorf("req.Filename=%q want %q", rec.got.Filename, want)
	}
	if rec.got.Base != "01-08-00-27-00-00-02" {
		t.Errorf("req.Base=%q want %q", rec.got.Base, "01-08-00-27-00-00-02")
	}
}

// ---------- Router ----------

func TestRouter(t *testing.T) {
	errBoom := errors.New("boom")
	tests := map[string]struct {
		routes     []*stubRoute
		wantCalled []bool
		wantErr    bool
		wantErrIs  error
		wantNotFnd bool // expect os.ErrNotExist wrap
	}{
		"first route handles → later routes skipped": {
			routes: []*stubRoute{
				{name: "first", handled: true},
				{name: "second"},
			},
			wantCalled: []bool{true, false},
		},
		"first not handled → second tries": {
			routes: []*stubRoute{
				{name: "first"},
				{name: "second", handled: true},
			},
			wantCalled: []bool{true, true},
		},
		"handled route's error is returned": {
			routes: []*stubRoute{
				{name: "first", handled: true, err: errBoom},
			},
			wantCalled: []bool{true},
			wantErr:    true,
			wantErrIs:  errBoom,
		},
		"no routes match → 404 wrap of os.ErrNotExist": {
			routes: []*stubRoute{
				{name: "first"},
				{name: "second"},
			},
			wantCalled: []bool{true, true},
			wantErr:    true,
			wantNotFnd: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			routes := make([]Route, len(tt.routes))
			for i, r := range tt.routes {
				routes[i] = r
			}
			r := Router{Log: logr.Discard(), Routes: routes}
			err := r.Handle(context.Background(), Request{Filename: "x", Base: "x"}, &captureWriter{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("expected error %v, got %v", tt.wantErrIs, err)
				}
				if tt.wantNotFnd && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("expected os.ErrNotExist wrap, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for i, want := range tt.wantCalled {
				if tt.routes[i].called != want {
					t.Errorf("route %d (%q): called=%v want=%v", i, tt.routes[i].name, tt.routes[i].called, want)
				}
			}
		})
	}
}

// ---------- EmbeddedIPXERoute ----------

func TestEmbeddedIPXERoute(t *testing.T) {
	t.Run("known binary served", func(t *testing.T) {
		r := EmbeddedIPXERoute{Log: logr.Discard()}
		w := &captureWriter{}
		handled, err := r.TryServe(context.Background(), Request{Filename: "undionly.kpxe", Base: "undionly.kpxe"}, w)
		if !handled {
			t.Fatal("expected handled=true for embedded binary")
		}
		if err != nil {
			t.Fatal(err)
		}
		if w.buf.Len() == 0 {
			t.Fatal("expected bytes written")
		}
	})

	t.Run("unknown file passes through", func(t *testing.T) {
		r := EmbeddedIPXERoute{Log: logr.Discard()}
		w := &captureWriter{}
		handled, err := r.TryServe(context.Background(), Request{Filename: "nope.bin", Base: "nope.bin"}, w)
		if handled {
			t.Fatal("expected handled=false for unknown")
		}
		if err != nil {
			t.Fatal(err)
		}
		if w.buf.Len() != 0 {
			t.Fatal("expected nothing written")
		}
	})
}

// ---------- DiskAssetRoute ----------

func TestDiskAssetRoute(t *testing.T) {
	dir := t.TempDir()
	body := "disk contents"
	if err := os.WriteFile(filepath.Join(dir, "snp.efi"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		dir         string
		filename    string
		wantHandled bool
		wantBody    string
	}{
		"empty dir passes through": {
			dir:         "",
			filename:    "snp.efi",
			wantHandled: false,
		},
		"existing file served": {
			dir:         dir,
			filename:    "snp.efi",
			wantHandled: true,
			wantBody:    body,
		},
		"missing file passes through": {
			dir:         dir,
			filename:    "missing.bin",
			wantHandled: false,
		},
		"traversal with .. is rejected": {
			dir:         dir,
			filename:    "../../etc/passwd",
			wantHandled: false,
		},
		"absolute path is rejected": {
			dir:         dir,
			filename:    "/etc/passwd",
			wantHandled: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := DiskAssetRoute{Log: logr.Discard(), Dir: tt.dir}
			w := &captureWriter{}
			handled, err := r.TryServe(context.Background(), Request{Filename: tt.filename}, w)
			if err != nil {
				t.Fatal(err)
			}
			if handled != tt.wantHandled {
				t.Fatalf("handled=%v want=%v", handled, tt.wantHandled)
			}
			if tt.wantBody != "" && w.buf.String() != tt.wantBody {
				t.Fatalf("body=%q want=%q", w.buf.String(), tt.wantBody)
			}
		})
	}
}
