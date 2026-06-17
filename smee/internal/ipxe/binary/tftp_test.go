package binary

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/smee/internal/hardware"
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

// fakeResolver is a minimal hardware.Resolver for tests. ByMAC keys on
// mac.String(); ByIP keys on ip.String(). If err is set, both methods
// return it.
type fakeResolver struct {
	byMAC map[string]hardware.Info
	byIP  map[string]hardware.Info
	err   error
}

func (f *fakeResolver) ByMAC(_ context.Context, m net.HardwareAddr) (hardware.Info, error) {
	if f.err != nil {
		return hardware.Info{}, f.err
	}
	if info, ok := f.byMAC[m.String()]; ok {
		return info, nil
	}
	return hardware.Info{}, fmt.Errorf("mac %s not found", m.String())
}

func (f *fakeResolver) ByIP(_ context.Context, ip net.IP) (hardware.Info, error) {
	if f.err != nil {
		return hardware.Info{}, f.err
	}
	if info, ok := f.byIP[ip.String()]; ok {
		return info, nil
	}
	return hardware.Info{}, fmt.Errorf("ip %s not found", ip.String())
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

// ---------- PXELinuxMACRoute ----------

func TestPXELinuxMACRoute(t *testing.T) {
	mac := net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	dashed := "aa-bb-cc-dd-ee-ff"

	tests := map[string]struct {
		filename       string
		resolver       hardware.Resolver
		wantHandled    bool
		wantBytesMatch string
	}{
		"valid path, template served": {
			filename: "pxelinux.cfg/01-" + dashed,
			resolver: &fakeResolver{byMAC: map[string]hardware.Info{
				mac.String(): {PXELINUX: hardware.PXELINUX{Config: "PROMPT 0\nDEFAULT linux"}},
			}},
			wantHandled:    true,
			wantBytesMatch: "PROMPT 0\nDEFAULT linux",
		},
		"wrong length passes through": {
			filename:    "pxelinux.cfg/01-short",
			resolver:    &fakeResolver{},
			wantHandled: false,
		},
		"different prefix passes through": {
			filename:    "other.cfg/01-aa-bb-cc-dd-ee-ff-XX",
			resolver:    &fakeResolver{},
			wantHandled: false,
		},
		"malformed MAC swallowed → not handled": {
			filename:    "pxelinux.cfg/01-ZZ-ZZ-ZZ-ZZ-ZZ-ZZ",
			resolver:    &fakeResolver{},
			wantHandled: false,
		},
		"resolver miss swallowed → not handled": {
			filename:    "pxelinux.cfg/01-" + dashed,
			resolver:    &fakeResolver{},
			wantHandled: false,
		},
		"empty template passes through": {
			filename: "pxelinux.cfg/01-" + dashed,
			resolver: &fakeResolver{byMAC: map[string]hardware.Info{
				mac.String(): {PXELINUX: hardware.PXELINUX{Config: ""}},
			}},
			wantHandled: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := PXELinuxMACRoute{Log: logr.Discard(), Resolver: tt.resolver}
			w := &captureWriter{}
			handled, err := r.TryServe(context.Background(), Request{Filename: tt.filename}, w)
			if err != nil {
				t.Fatal(err)
			}
			if handled != tt.wantHandled {
				t.Fatalf("handled=%v want=%v", handled, tt.wantHandled)
			}
			if tt.wantBytesMatch != "" && w.buf.String() != tt.wantBytesMatch {
				t.Fatalf("body=%q want=%q", w.buf.String(), tt.wantBytesMatch)
			}
		})
	}
}

// ---------- RPiNetbootRoute ----------

func TestRPiNetbootRoute(t *testing.T) {
	clientIP := net.ParseIP("192.168.1.50")
	clientAddr := net.UDPAddr{IP: clientIP, Port: 12345}

	const serial = "abc123"
	const rewrite = "rpi4b"

	hwWithRPi := hardware.Info{
		RPI: hardware.RPI{
			SerialNum:    serial,
			FirmwarePath: rewrite,
			ConfigTxt:    "config-txt-body",
		},
		OSIE: hardware.OSIE{
			KernelParams: []string{"console=tty1", "rw"},
		},
	}

	// Set up an asset dir with a known file
	assetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(assetDir, rewrite), 0o755); err != nil {
		t.Fatal(err)
	}
	const assetBody = "start.elf contents"
	if err := os.WriteFile(filepath.Join(assetDir, rewrite, "start.elf"), []byte(assetBody), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		filename    string
		assetDir    string
		resolver    hardware.Resolver
		wantHandled bool
		wantBody    string
	}{
		"empty AssetDir passes through": {
			filename:    serial + "/config.txt",
			assetDir:    "",
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: false,
		},
		"ByIP miss passes through": {
			filename:    serial + "/config.txt",
			assetDir:    assetDir,
			resolver:    &fakeResolver{},
			wantHandled: false,
		},
		"hardware without RPi config passes through": {
			filename: serial + "/config.txt",
			assetDir: assetDir,
			resolver: &fakeResolver{byIP: map[string]hardware.Info{
				clientIP.String(): {},
			}},
			wantHandled: false,
		},
		"non-serial-prefixed path passes through": {
			filename:    "other-serial/config.txt",
			assetDir:    assetDir,
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: false,
		},
		"config.txt served from ConfigTxt": {
			filename:    serial + "/config.txt",
			assetDir:    assetDir,
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: true,
			wantBody:    "config-txt-body",
		},
		"cmdline.txt served from joined OSIE.KernelParams": {
			filename:    serial + "/cmdline.txt",
			assetDir:    assetDir,
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: true,
			wantBody:    "console=tty1 rw",
		},
		"empty KernelParams cmdline.txt passes through": {
			filename: serial + "/cmdline.txt",
			assetDir: assetDir,
			resolver: &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): {
				RPI: hardware.RPI{SerialNum: serial, FirmwarePath: rewrite},
				// no KernelParams
			}}},
			wantHandled: false,
		},
		"other file rewritten and served from disk": {
			filename:    serial + "/start.elf",
			assetDir:    assetDir,
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: true,
			wantBody:    assetBody,
		},
		"rewritten path miss passes through": {
			filename:    serial + "/missing.bin",
			assetDir:    assetDir,
			resolver:    &fakeResolver{byIP: map[string]hardware.Info{clientIP.String(): hwWithRPi}},
			wantHandled: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := RPiNetbootRoute{Log: logr.Discard(), Resolver: tt.resolver, AssetDir: tt.assetDir}
			w := &captureWriter{}
			handled, err := r.TryServe(context.Background(), Request{Filename: tt.filename, Client: clientAddr}, w)
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
