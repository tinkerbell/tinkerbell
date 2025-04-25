package build

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		version     string
		want        *pseudoVersion
		wantErr     bool
		errContains string
	}{
		"nobase form": {
			version: "v0.0.0-20230405123456-abcdef123456",
			want: &pseudoVersion{
				BaseVersion: "v0.0.0",
				CommitHash:  "abcdef12",
				IsDirty:     false,
			},
		},
		"nobase form with dirty": {
			version: "v0.0.0-20230405123456-abcdef123456+dirty",
			want: &pseudoVersion{
				BaseVersion: "v0.0.0",
				CommitHash:  "abcdef12",
				IsDirty:     true,
			},
		},
		"pre-release form": {
			version: "v1.2.3-pre.0.20230405123456-abcdef123456",
			want: &pseudoVersion{
				BaseVersion: "v1.2.3-pre",
				CommitHash:  "abcdef12",
				IsDirty:     false,
			},
		},
		"release form": {
			version: "v1.2.3-0.20230405123456-abcdef123456",
			want: &pseudoVersion{
				BaseVersion: "v1.2.3",
				CommitHash:  "abcdef12",
				IsDirty:     false,
			},
		},
		"invalid format": {
			version:     "not-a-version",
			wantErr:     true,
			errContains: "invalid pseudo-version format",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parse(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Error("parse() error = nil, wantErr true")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("parse() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("parse() unexpected error = %v", err)
				return
			}
			if got.BaseVersion != tt.want.BaseVersion {
				t.Errorf("parse() BaseVersion = %v, want %v", got.BaseVersion, tt.want.BaseVersion)
			}
			if got.CommitHash != tt.want.CommitHash {
				t.Errorf("parse() CommitHash = %v, want %v", got.CommitHash, tt.want.CommitHash)
			}
			if got.IsDirty != tt.want.IsDirty {
				t.Errorf("parse() IsDirty = %v, want %v", got.IsDirty, tt.want.IsDirty)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	tests := map[string]struct {
		raw     string
		want    string
		wantRaw bool
	}{
		"valid version": {
			raw:  "v1.2.3-0.20230405123456-abcdef123456",
			want: "v1.2.3-abcdef12",
		},
		"invalid version returns raw": {
			raw:     "invalid-version",
			want:    "invalid-version",
			wantRaw: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := Version(tt.raw)
			if got != tt.want {
				t.Errorf("Version() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPseudoVersionString(t *testing.T) {
	tests := map[string]struct {
		pv   *pseudoVersion
		want string
	}{
		"clean version": {
			pv: &pseudoVersion{
				BaseVersion: "v1.2.3",
				CommitHash:  "abcdef12",
				IsDirty:     false,
			},
			want: "v1.2.3-abcdef12",
		},
		"dirty version": {
			pv: &pseudoVersion{
				BaseVersion: "v1.2.3",
				CommitHash:  "abcdef12",
				IsDirty:     true,
			},
			want: "v1.2.3-abcdef12+dirty",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.pv.String(); got != tt.want {
				t.Errorf("pseudoVersion.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
