package script

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestPxeLinuxPattern(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		wantMatch  bool
		wantHWType string
		wantMAC    string
	}{
		{
			name:       "valid ethernet with colons",
			filename:   "pxelinux.cfg/01-00:11:22:33:44:55",
			wantMatch:  true,
			wantHWType: "01",
			wantMAC:    "00:11:22:33:44:55",
		},
		{
			name:       "valid ethernet with dashes",
			filename:   "pxelinux.cfg/01-00-11-22-33-44-55",
			wantMatch:  true,
			wantHWType: "01",
			wantMAC:    "00-11-22-33-44-55",
		},
		{
			name:       "valid token ring (06)",
			filename:   "pxelinux.cfg/06-00:11:22:33:44:55",
			wantMatch:  true,
			wantHWType: "06",
			wantMAC:    "00:11:22:33:44:55",
		},
		{
			name:       "valid with uppercase hex",
			filename:   "pxelinux.cfg/0A-AA:BB:CC:DD:EE:FF",
			wantMatch:  true,
			wantHWType: "0A",
			wantMAC:    "AA:BB:CC:DD:EE:FF",
		},
		{
			name:      "invalid - missing hardware type",
			filename:  "pxelinux.cfg/00:11:22:33:44:55",
			wantMatch: false,
		},
		{
			name:      "invalid - single digit hardware type",
			filename:  "pxelinux.cfg/1-00:11:22:33:44:55",
			wantMatch: false,
		},
		{
			name:      "invalid - three digit hardware type",
			filename:  "pxelinux.cfg/001-00:11:22:33:44:55",
			wantMatch: false,
		},
		{
			name:      "invalid - non-hex hardware type",
			filename:  "pxelinux.cfg/ZZ-00:11:22:33:44:55",
			wantMatch: false,
		},
		{
			name:      "invalid - wrong prefix",
			filename:  "ipxe.cfg/01-00:11:22:33:44:55",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := pxeLinuxPattern.FindStringSubmatch(tt.filename)

			if tt.wantMatch {
				if matches == nil || len(matches) != 3 {
					t.Errorf("expected pattern to match, but it didn't. filename=%s, matches=%v", tt.filename, matches)
					return
				}

				gotHWType := matches[1]
				gotMAC := matches[2]

				if gotHWType != tt.wantHWType {
					t.Errorf("hardware type mismatch: got=%s, want=%s", gotHWType, tt.wantHWType)
				}

				if gotMAC != tt.wantMAC {
					t.Errorf("MAC address mismatch: got=%s, want=%s", gotMAC, tt.wantMAC)
				}
			} else if len(matches) == 3 {
				t.Errorf("expected pattern not to match, but it did. filename=%s, matches=%v", tt.filename, matches)
			}
		})
	}
}

func TestHandleTFTP_FilenameValidation(t *testing.T) {
	// This test verifies that HandleTFTP properly validates filenames
	// without needing a full handler setup

	validFilenames := []string{
		"pxelinux.cfg/01-00:11:22:33:44:55",
		"pxelinux.cfg/01-00-11-22-33-44-55",
		"pxelinux.cfg/06-aa:bb:cc:dd:ee:ff",
		"pxelinux.cfg/0a-AA:BB:CC:DD:EE:FF",
	}

	invalidFilenames := []string{
		"pxelinux.cfg/00:11:22:33:44:55",     // missing hwtype
		"pxelinux.cfg/1-00:11:22:33:44:55",   // single digit hwtype
		"pxelinux.cfg/001-00:11:22:33:44:55", // three digit hwtype
		"ipxe.cfg/01-00:11:22:33:44:55",      // wrong prefix
		"pxelinux.cfg/ZZ-00:11:22:33:44:55",  // non-hex hwtype
	}

	for _, filename := range validFilenames {
		t.Run("valid_"+filename, func(t *testing.T) {
			matches := pxeLinuxPattern.FindStringSubmatch(filename)
			if matches == nil || len(matches) != 3 {
				t.Errorf("valid filename should match pattern: %s", filename)
			}
		})
	}

	for _, filename := range invalidFilenames {
		t.Run("invalid_"+filename, func(t *testing.T) {
			matches := pxeLinuxPattern.FindStringSubmatch(filename)
			if len(matches) == 3 {
				t.Errorf("invalid filename should not match pattern: %s", filename)
			}
		})
	}
}

func TestHandleTFTP_ErrorMessages(t *testing.T) {
	// Test that the error message format is correct for invalid filenames
	h := Handler{
		Logger: logr.Discard(),
	}

	tests := []struct {
		name          string
		filename      string
		wantErrSubstr string
	}{
		{
			name:          "missing hardware type",
			filename:      "pxelinux.cfg/00:11:22:33:44:55",
			wantErrSubstr: "invalid pxelinux config filename format",
		},
		{
			name:          "wrong prefix",
			filename:      "ipxe.cfg/01-00:11:22:33:44:55",
			wantErrSubstr: "invalid pxelinux config filename format",
		},
		{
			name:          "invalid MAC with valid format",
			filename:      "pxelinux.cfg/01-invalid-mac",
			wantErrSubstr: "invalid MAC address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.HandleTFTP(tt.filename, nil)
			if err == nil {
				t.Errorf("expected error for filename: %s", tt.filename)
				return
			}

			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error message should contain %q, got: %s", tt.wantErrSubstr, err.Error())
			}
		})
	}
}
