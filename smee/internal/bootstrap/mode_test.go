package bootstrap

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/crd"
)

var parseModeTests = map[string]struct {
	in      string
	want    uint32
	wantErr bool
}{
	"unset is the default":      {"", DefaultMode, false},
	"whitespace is the default": {"   ", DefaultMode, false},
	"bare octal":                {"644", 0o644, false},
	"leading zero":              {"0644", 0o644, false},
	"many leading zeros":        {"000644", 0o644, false},
	"go style prefix":           {"0o644", 0o644, false},
	"upper case prefix":         {"0O644", 0o644, false},
	"executable":                {"755", 0o755, false},
	"owner only":                {"400", 0o400, false},
	"all permissions":           {"777", 0o777, false},
	"explicit none":             {"0", 0, false},
	"bare prefix":               {"0o", 0, true},
	"decimal digit 8":           {"888", 0, true},
	"decimal digit 9":           {"999", 0, true},
	"not a number":              {"rwxr-xr-x", 0, true},
	"hex prefix":                {"0x1a4", 0, true},
	"negative":                  {"-644", 0, true},
	"setuid is rejected":        {"4755", 0, true},
	"setgid is rejected":        {"2755", 0, true},
	"sticky is rejected":        {"1777", 0, true},
	"too many digits":           {"77777", 0, true},
}

func TestParseMode(t *testing.T) {
	for name, tt := range parseModeTests {
		t.Run(name, func(t *testing.T) {
			got, err := parseMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got error %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("got %o, want %o", got, tt.want)
			}
		})
	}
}

// TestCRDModePatternMatchesParseMode keeps API admission and parseMode in
// agreement: a mode the API server admits must never fail at boot time.
func TestCRDModePatternMatchesParseMode(t *testing.T) {
	var c apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(crd.TinkerbellDefaults["hardware.tinkerbell.org"], &c); err != nil {
		t.Fatal(err)
	}
	mode := c.Spec.Versions[0].Schema.OpenAPIV3Schema.
		Properties["spec"].Properties["metadata"].Properties["osieFiles"].Items.Schema.Properties["mode"]
	if mode.Pattern == "" {
		t.Fatal("CRD has no pattern for spec.metadata.osieFiles[].mode")
	}
	pattern := regexp.MustCompile(mode.Pattern)

	for name, tt := range parseModeTests {
		// The API omits empty strings, and only Smee trims whitespace.
		if strings.TrimSpace(tt.in) != tt.in || tt.in == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			admitted := pattern.MatchString(tt.in) && int64(len(tt.in)) <= *mode.MaxLength
			if admitted == tt.wantErr {
				t.Errorf("CRD admits %q = %v, but parseMode error = %v", tt.in, admitted, tt.wantErr)
			}
		})
	}
}

// TestFilesAppliesMode covers the mode reaching the archive, including the
// default for entries that omit it.
func TestFilesAppliesMode(t *testing.T) {
	hw := hardwareWithFiles(
		tinkerbell.OSIEFile{Path: "certificates/ca.pem", Contents: "cert"},
		tinkerbell.OSIEFile{Path: "scripts/setup.sh", Contents: "#!/bin/sh\n", Mode: "755"},
		tinkerbell.OSIEFile{Path: "shared.conf", Contents: "x", Mode: "0644"},
	)
	files, err := Files(hw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Archive(files, ModTime(hw))
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]uint32{
		Dir + "/certificates/ca.pem": typeFile | DefaultMode,
		Dir + "/scripts/setup.sh":    typeFile | 0o755,
		Dir + "/shared.conf":         typeFile | 0o644,
		// Directories stay traversable regardless of how restrictive the files are.
		Dir:                   typeDir | 0o755,
		Dir + "/certificates": typeDir | 0o755,
		Dir + "/scripts":      typeDir | 0o755,
	}
	for _, e := range readArchive(t, a) {
		w, ok := want[e.name]
		if !ok {
			t.Errorf("unexpected entry %s", e.name)
			continue
		}
		if e.mode != w {
			t.Errorf("%s mode = %o, want %o", e.name, e.mode, w)
		}
		delete(want, e.name)
	}
	for name := range want {
		t.Errorf("missing entry %s", name)
	}
}

func TestArchiveRejectsInvalidMode(t *testing.T) {
	for _, mode := range []uint32{0o1000, 0o2755, 0o4755, 0o10644, 0xFFFFFFFF} {
		if _, err := Archive([]File{{Name: "a.txt", Mode: mode}}, time.Time{}); err == nil {
			t.Errorf("mode %#o: got nil error, want rejection", mode)
		}
	}
}

func TestHandlerRejectsInvalidMode(t *testing.T) {
	hw := hardwareWithFiles(tinkerbell.OSIEFile{Path: "a.txt", Contents: "x", Mode: "not-octal"})
	resp := serve(t, &stubBackend{hw: hw}, http.MethodGet, "/bootstrap/"+testMAC+"/"+ArchiveName)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}
