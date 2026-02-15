package containerd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

func TestParseVolume(t *testing.T) {
	log := logr.Discard()

	tests := map[string]struct {
		volume string
		want   *specs.Mount
	}{
		"absolute source and destination with default options": {
			volume: "/host/path:/container/path",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "rw"},
			},
		},
		"read-only option": {
			volume: "/host/path:/container/path:ro",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "ro"},
			},
		},
		"explicit read-write option": {
			volume: "/host/path:/container/path:rw",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "rw"},
			},
		},
		"multiple options": {
			volume: "/host/path:/container/path:ro,noexec",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "noexec", "ro"},
			},
		},
		"relative source with dot prefix": {
			volume: "./relative:/container/path",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "./relative",
				Destination: "/container/path",
				Options:     []string{"rbind", "rw"},
			},
		},
		"named volume skipped": {
			volume: "myvolume:/container/path",
			want:   nil,
		},
		"missing destination": {
			volume: "/host/path",
			want:   nil,
		},
		"empty string": {
			volume: "",
			want:   nil,
		},
		"empty destination": {
			volume: "/host/path:",
			want:   nil,
		},
		"relative destination rejected": {
			volume: "/host/path:relative/path",
			want:   nil,
		},
		"custom option without ro or rw gets rw appended": {
			volume: "/host/path:/container/path:noexec",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "noexec", "rw"},
			},
		},
		"empty option parts are filtered": {
			volume: "/host/path:/container/path:ro,",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "ro"},
			},
		},
		"options with whitespace trimmed": {
			volume: "/host/path:/container/path: ro , noexec ",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "noexec", "ro"},
			},
		},
		"conflicting ro and rw uses last wins": {
			volume: "/host/path:/container/path:ro,rw",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "rw"},
			},
		},
		"whitespace-only option is filtered": {
			volume: "/host/path:/container/path: ",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "rw"},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseVolume(log, tt.volume)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("parseVolume(%q) = %+v, want nil", tt.volume, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseVolume(%q) = nil, want %+v", tt.volume, tt.want)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.want.Type)
			}
			if got.Source != tt.want.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.want.Source)
			}
			if got.Destination != tt.want.Destination {
				t.Errorf("Destination = %q, want %q", got.Destination, tt.want.Destination)
			}
			if len(got.Options) != len(tt.want.Options) {
				t.Fatalf("Options = %v, want %v", got.Options, tt.want.Options)
			}
			for i := range got.Options {
				if got.Options[i] != tt.want.Options[i] {
					t.Errorf("Options[%d] = %q, want %q", i, got.Options[i], tt.want.Options[i])
				}
			}
		})
	}
}

func TestParseVolumes(t *testing.T) {
	log := logr.Discard()

	t.Run("nil volumes", func(t *testing.T) {
		got := parseVolumes(log, nil)
		if len(got) != 0 {
			t.Errorf("parseVolumes(nil) returned %d mounts, want 0", len(got))
		}
	})

	t.Run("empty volumes", func(t *testing.T) {
		got := parseVolumes(log, []spec.Volume{})
		if len(got) != 0 {
			t.Errorf("parseVolumes([]) returned %d mounts, want 0", len(got))
		}
	})

	t.Run("single valid volume with existing source", func(t *testing.T) {
		src := t.TempDir()
		vols := []spec.Volume{spec.Volume(src + ":/container")}
		got := parseVolumes(log, vols)
		if len(got) != 1 {
			t.Fatalf("parseVolumes() returned %d mounts, want 1", len(got))
		}
		if got[0].Source != src {
			t.Errorf("Source = %q, want %q", got[0].Source, src)
		}
	})

	t.Run("creates missing source directory", func(t *testing.T) {
		base := t.TempDir()
		src := filepath.Join(base, "nonexistent", "deep")
		vols := []spec.Volume{spec.Volume(src + ":/container")}
		got := parseVolumes(log, vols)
		if len(got) != 1 {
			t.Fatalf("parseVolumes() returned %d mounts, want 1", len(got))
		}
		if got[0].Source != src {
			t.Errorf("Source = %q, want %q", got[0].Source, src)
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatalf("source directory was not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("source path is not a directory")
		}
	})

	t.Run("multiple valid volumes", func(t *testing.T) {
		src1 := t.TempDir()
		src2 := t.TempDir()
		vols := []spec.Volume{
			spec.Volume(src1 + ":/container1"),
			spec.Volume(src2 + ":/container2:ro"),
		}
		got := parseVolumes(log, vols)
		if len(got) != 2 {
			t.Errorf("parseVolumes() returned %d mounts, want 2", len(got))
		}
	})

	t.Run("mix of valid and invalid volumes", func(t *testing.T) {
		src1 := t.TempDir()
		base := t.TempDir()
		src3 := filepath.Join(base, "newdir")
		vols := []spec.Volume{
			spec.Volume(src1 + ":/container"),
			"namedvol:/container2",
			spec.Volume(src3 + ":/container3:rw"),
		}
		got := parseVolumes(log, vols)
		if len(got) != 2 {
			t.Errorf("parseVolumes() returned %d mounts, want 2", len(got))
		}
	})

	t.Run("all invalid volumes", func(t *testing.T) {
		vols := []spec.Volume{
			"namedvol:/data",
			"invalidformat",
		}
		got := parseVolumes(log, vols)
		if len(got) != 0 {
			t.Errorf("parseVolumes() returned %d mounts, want 0", len(got))
		}
	})

	t.Run("resolves relative source to absolute", func(t *testing.T) {
		// Create a temp dir and chdir into it so relative paths resolve there.
		base := t.TempDir()
		orig, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chdir(orig) })
		if err := os.Chdir(base); err != nil {
			t.Fatal(err)
		}
		vols := []spec.Volume{"./reldir:/container"}
		got := parseVolumes(log, vols)
		if len(got) != 1 {
			t.Fatalf("parseVolumes() returned %d mounts, want 1", len(got))
		}
		want := filepath.Join(base, "reldir")
		if got[0].Source != want {
			t.Errorf("Source = %q, want %q", got[0].Source, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("relative source directory was not created: %v", err)
		}
	})
}

func TestEnsureBindMountSource(t *testing.T) {
	t.Run("existing directory returns path unchanged", func(t *testing.T) {
		dir := t.TempDir()
		got, err := ensureBindMountSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dir {
			t.Errorf("got %q, want %q", got, dir)
		}
	})

	t.Run("nonexistent path creates directory", func(t *testing.T) {
		base := t.TempDir()
		src := filepath.Join(base, "a", "b", "c")
		got, err := ensureBindMountSource(src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != src {
			t.Errorf("got %q, want %q", got, src)
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatalf("directory was not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("path is not a directory")
		}
		// Verify permissions (0o755).
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("permissions = %o, want 755", perm)
		}
	})

	t.Run("relative path resolved to absolute", func(t *testing.T) {
		base := t.TempDir()
		orig, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chdir(orig) })
		if err := os.Chdir(base); err != nil {
			t.Fatal(err)
		}
		got, err := ensureBindMountSource("./mydir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(base, "mydir")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("directory was not created: %v", err)
		}
	})

	t.Run("existing file returns path as-is", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ensureBindMountSource(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != f {
			t.Errorf("got %q, want %q", got, f)
		}
	})
}
