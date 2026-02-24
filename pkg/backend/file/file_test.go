package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func TestNewWatcher(t *testing.T) {
	tests := map[string]struct {
		createFile bool
		want       string
		wantErr    error
	}{
		"contents equal": {createFile: true, want: "test content here"},
		"file not found": {createFile: false, wantErr: &fs.PathError{}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var name string
			if tt.createFile {
				var err error
				name, err = createFile([]byte(tt.want))
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(name)
			}
			w, err := NewWatcher(logr.Discard(), name)
			if (err != nil) != (tt.wantErr != nil) {
				t.Fatalf("NewWatcher() error = %v; type = %[1]T, wantErr %v; type = %[2]T", err, tt.wantErr)
			}
			var got string
			if tt.wantErr != nil {
				got = ""
			} else {
				got = string(w.data)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func createFile(content []byte) (string, error) {
	file, err := os.CreateTemp("", "prefix")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return "", err
	}
	return file.Name(), nil
}

type testData struct {
	initial     string
	after       string
	action      string
	expectedOut string
}

// noTime removes the time key from the slog output.
var noTime = &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}}

func TestStartAndStop(t *testing.T) {
	tt := &testData{action: "cancel", expectedOut: `{"level":"INFO","msg":"stopping watcher"}` + "\n"}
	out := &bytes.Buffer{}
	l := logr.FromSlogHandler(slog.NewJSONHandler(out, noTime))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w := &Watcher{Log: l, watcher: watcher}
	w.Start(ctx)
	if diff := cmp.Diff(out.String(), tt.expectedOut); diff != "" {
		t.Fatal(diff)
	}
}

func TestStartFileUpdateError(t *testing.T) {
	expected := `{"level":"INFO","msg":"file changed, updating cache"}
{"level":"ERROR","msg":"failed to read file","err":"open not-found.txt: no such file or directory","file":"not-found.txt"}
{"level":"INFO","msg":"stopping watcher"}
`
	tt := &testData{expectedOut: expected}
	out := &bytes.Buffer{}
	l := logr.FromSlogHandler(slog.NewJSONHandler(out, noTime))
	got, name := tt.helper(t, l)
	defer os.Remove(name)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(time.Millisecond)
		got.FilePath = "not-found.txt"
		got.watcher.Events <- fsnotify.Event{Op: fsnotify.Write}
		cancel()
	}()
	got.Start(ctx)
	time.Sleep(time.Second)
	if diff := cmp.Diff(out.String(), tt.expectedOut); diff != "" {
		t.Log(out.String())
		t.Fatal(diff)
	}
}

func TestStartFileUpdate(t *testing.T) {
	tt := &testData{initial: "once upon a time", after: "\nhello world", expectedOut: "once upon a time\nhello world"}
	got, name := tt.helper(t, logr.Discard())
	defer os.Remove(name)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(time.Millisecond)
		got.fileMu.Lock()
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Log(err)
		}
		f.Write([]byte(tt.after))
		f.Close()
		got.fileMu.Unlock()
		time.Sleep(time.Millisecond)
		cancel()
	}()
	got.Start(ctx)
	got.dataMu.RLock()
	d := got.data
	got.dataMu.RUnlock()
	if diff := cmp.Diff(string(d), tt.expectedOut); diff != "" {
		t.Log(string(d))
		t.Fatal(diff)
	}
}

func TestStartFileUpdateClosedChan(t *testing.T) {
	out := &bytes.Buffer{}
	l := logr.FromSlogHandler(slog.NewJSONHandler(out, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w := &Watcher{Log: l, watcher: watcher}
	go w.Start(ctx)
	close(w.watcher.Events)
	time.Sleep(time.Millisecond)
	if diff := cmp.Diff(out.String(), ""); diff != "" {
		t.Fatal(diff)
	}
}

func TestStartError(t *testing.T) {
	expected := `{"level":"INFO","msg":"error watching file","err":"test error"}
{"level":"INFO","msg":"stopping watcher"}
`
	tt := &testData{expectedOut: expected}
	out := &bytes.Buffer{}
	l := logr.FromSlogHandler(slog.NewJSONHandler(out, noTime))
	ctx, cancel := context.WithCancel(context.Background())
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w := &Watcher{Log: l, watcher: watcher}
	go func() {
		time.Sleep(time.Millisecond)
		w.watcher.Errors <- fmt.Errorf("test error")
		cancel()
	}()
	w.Start(ctx)
	if diff := cmp.Diff(out.String(), tt.expectedOut); diff != "" {
		t.Fatal(diff)
	}
}

func TestStartErrorContinue(t *testing.T) {
	out := &bytes.Buffer{}
	l := logr.FromSlogHandler(slog.NewJSONHandler(out, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w := &Watcher{Log: l, watcher: watcher}
	go w.Start(ctx)
	close(w.watcher.Errors)
	time.Sleep(time.Millisecond)
	if diff := cmp.Diff(out.String(), ""); diff != "" {
		t.Fatal(diff)
	}
}

func (tt *testData) helper(t *testing.T, l logr.Logger) (*Watcher, string) {
	t.Helper()
	name, err := createFile([]byte(tt.initial))
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(l, name)
	if err != nil {
		t.Fatal(err)
	}
	w.dataMu.RLock()
	before := string(w.data)
	w.dataMu.RUnlock()
	if diff := cmp.Diff(before, tt.initial); diff != "" {
		t.Fatal("before", diff)
	}

	return w, name
}

func TestReadHardwareByMac(t *testing.T) {
	tests := map[string]struct {
		mac     string
		badData bool
		wantErr error
	}{
		"no record found":   {mac: "00:01:02:03:04:05", wantErr: errRecordNotFound},
		"record found":      {mac: "08:00:27:29:4e:67", wantErr: nil},
		"fail parsing file": {badData: true, wantErr: errFileFormat},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dataFile := "testdata/example.yaml"
			if tt.badData {
				var err error
				dataFile, err = createFile([]byte("not a yaml file"))
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(dataFile)
			}
			w, err := NewWatcher(logr.Discard(), dataFile)
			if err != nil {
				t.Fatal(err)
			}
			hw, err := w.ReadHardware(context.Background(), "", "", data.ReadListOptions{Hardware: data.HardwareReadOptions{ByMACAddress: tt.mac}})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadHardware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && hw == nil {
				t.Fatal("expected hardware, got nil")
			}
		})
	}
}

func TestReadHardwareByIP(t *testing.T) {
	tests := map[string]struct {
		ip      string
		badData bool
		wantErr error
	}{
		"no record found":   {ip: "172.168.2.1", wantErr: errRecordNotFound},
		"record found":      {ip: "192.168.2.153", wantErr: nil},
		"fail parsing file": {badData: true, wantErr: errFileFormat},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dataFile := "testdata/example.yaml"
			if tt.badData {
				var err error
				dataFile, err = createFile([]byte("not a yaml file"))
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(dataFile)
			}
			w, err := NewWatcher(logr.Discard(), dataFile)
			if err != nil {
				t.Fatal(err)
			}
			hw, err := w.ReadHardware(context.Background(), "", "", data.ReadListOptions{Hardware: data.HardwareReadOptions{ByIPAddress: tt.ip}})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadHardware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && hw == nil {
				t.Fatal("expected hardware, got nil")
			}
		})
	}
}

func TestReadHardwareByName(t *testing.T) {
	tests := map[string]struct {
		name    string
		wantErr error
	}{
		"no record found": {name: "nonexistent", wantErr: errRecordNotFound},
		"record found":    {name: "pxe-virtualbox", wantErr: nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w, err := NewWatcher(logr.Discard(), "testdata/example.yaml")
			if err != nil {
				t.Fatal(err)
			}
			hw, err := w.ReadHardware(context.Background(), "", "", data.ReadListOptions{ByName: tt.name})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadHardware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && hw == nil {
				t.Fatal("expected hardware, got nil")
			}
		})
	}
}
