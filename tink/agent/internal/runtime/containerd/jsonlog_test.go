package containerd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// decodeAll consumes the json-file at path into a slice of entries.
func decodeAll(t *testing.T, path string) []jsonLogEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []jsonLogEntry
	dec := json.NewDecoder(f)
	for {
		var e jsonLogEntry
		if err := dec.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func TestJSONLogPair_MultiLineWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(pair.Stdout, "alpha\nbeta\ngamma\n"); err != nil {
		t.Fatal(err)
	}
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}
	got := decodeAll(t, path)
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d (%+v)", len(got), got)
	}
	for i, want := range []string{"alpha\n", "beta\n", "gamma\n"} {
		if got[i].Log != want {
			t.Errorf("entry[%d].Log = %q, want %q", i, got[i].Log, want)
		}
		if got[i].Stream != "stdout" {
			t.Errorf("entry[%d].Stream = %q, want stdout", i, got[i].Stream)
		}
		if got[i].Time.IsZero() {
			t.Errorf("entry[%d].Time should not be zero", i)
		}
	}
}

func TestJSONLogPair_PartialLineBuffering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	// Write the line in three pieces; only one entry should appear, after
	// the newline finally arrives.
	io.WriteString(pair.Stdout, "hel")
	io.WriteString(pair.Stdout, "lo wor")
	io.WriteString(pair.Stdout, "ld\n")
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}
	got := decodeAll(t, path)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d (%+v)", len(got), got)
	}
	if got[0].Log != "hello world\n" {
		t.Errorf("Log = %q, want %q", got[0].Log, "hello world\n")
	}
}

func TestJSONLogPair_FlushUnterminatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	// No trailing newline — flush on Close must still emit the line.
	io.WriteString(pair.Stdout, "no-newline-here")
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}
	got := decodeAll(t, path)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d (%+v)", len(got), got)
	}
	if got[0].Log != "no-newline-here" {
		t.Errorf("Log = %q, want %q", got[0].Log, "no-newline-here")
	}
}

func TestJSONLogPair_StreamLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(pair.Stdout, "out\n")
	io.WriteString(pair.Stderr, "err\n")
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}
	got := decodeAll(t, path)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d (%+v)", len(got), got)
	}
	streams := []string{got[0].Stream, got[1].Stream}
	logs := []string{got[0].Log, got[1].Log}
	if streams[0] != "stdout" || streams[1] != "stderr" {
		t.Errorf("streams = %v, want [stdout stderr]", streams)
	}
	if logs[0] != "out\n" || logs[1] != "err\n" {
		t.Errorf("logs = %v", logs)
	}
}

// TestJSONLogPair_ConcurrentWrites makes sure interleaved writes from the
// stdout and stderr writers still produce well-formed JSONL (one decodable
// object per line). Without the shared mutex, partial-line buffering across
// two Encoders would corrupt the file.
func TestJSONLogPair_ConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	const n = 200
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			io.WriteString(pair.Stdout, "stdout line\n")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			io.WriteString(pair.Stderr, "stderr line\n")
		}
	}()
	wg.Wait()
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}

	got := decodeAll(t, path)
	if len(got) != 2*n {
		t.Fatalf("want %d entries, got %d", 2*n, len(got))
	}
	var outCount, errCount int
	for _, e := range got {
		switch e.Stream {
		case "stdout":
			outCount++
			if e.Log != "stdout line\n" {
				t.Errorf("stdout entry corrupt: %q", e.Log)
			}
		case "stderr":
			errCount++
			if e.Log != "stderr line\n" {
				t.Errorf("stderr entry corrupt: %q", e.Log)
			}
		default:
			t.Errorf("unknown stream %q", e.Stream)
		}
	}
	if outCount != n || errCount != n {
		t.Errorf("counts: stdout=%d stderr=%d, want %d each", outCount, errCount, n)
	}
}

// TestJSONLogPair_TeeShape verifies that combining the writer with another
// io.Writer via io.MultiWriter (as the runtime does with os.Stdout) gives
// both sinks the original bytes.
func TestJSONLogPair_TeeShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	pair, err := newJSONLogPair(path)
	if err != nil {
		t.Fatal(err)
	}
	var sideStdout bytes.Buffer
	w := io.MultiWriter(&sideStdout, pair.Stdout)
	io.WriteString(w, "tee one\ntee two\n")
	if err := pair.Close(); err != nil {
		t.Fatal(err)
	}

	if got := sideStdout.String(); got != "tee one\ntee two\n" {
		t.Errorf("side stdout = %q, want raw bytes", got)
	}
	got := decodeAll(t, path)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0].Log != "tee one\n" || got[1].Log != "tee two\n" {
		t.Errorf("entries = %+v", got)
	}
	// Sanity: each entry must carry a non-zero timestamp from "now".
	// Allow a small clock skew window in either direction.
	now := time.Now()
	for _, e := range got {
		if e.Time.IsZero() {
			t.Errorf("time is zero: %v", e.Time)
			continue
		}
		if d := now.Sub(e.Time); d < -time.Second || d > time.Minute {
			t.Errorf("time %v not within recent window of %v (delta %v)", e.Time, now, d)
		}
	}
}
