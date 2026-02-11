package containerd

import (
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
				Options:     []string{"rbind", "ro", "noexec"},
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
			volume: "/host/path:/container/path: ro , rw ",
			want: &specs.Mount{
				Type:        "bind",
				Source:      "/host/path",
				Destination: "/container/path",
				Options:     []string{"rbind", "ro", "rw"},
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

	tests := map[string]struct {
		volumes []spec.Volume
		want    int // expected number of mounts
	}{
		"nil volumes": {
			volumes: nil,
			want:    0,
		},
		"empty volumes": {
			volumes: []spec.Volume{},
			want:    0,
		},
		"single valid volume": {
			volumes: []spec.Volume{
				"/host:/container",
			},
			want: 1,
		},
		"multiple valid volumes": {
			volumes: []spec.Volume{
				"/host1:/container1",
				"/host2:/container2:ro",
			},
			want: 2,
		},
		"mix of valid and invalid volumes": {
			volumes: []spec.Volume{
				"/host:/container",
				"namedvol:/container2",
				"/host3:/container3:rw",
			},
			want: 2,
		},
		"all invalid volumes": {
			volumes: []spec.Volume{
				"namedvol:/data",
				"invalidformat",
			},
			want: 0,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseVolumes(log, tt.volumes)
			if len(got) != tt.want {
				t.Errorf("parseVolumes() returned %d mounts, want %d", len(got), tt.want)
			}
		})
	}
}
