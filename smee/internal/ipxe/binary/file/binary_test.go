package binary

import (
	"bytes"
	"testing"
)

func TestBinariesContainMagicString(t *testing.T) {
	for file, data := range Files {
		switch file {
		case "undionly.kpxe", "snp-arm64.efi", "snp-x86_64.efi":
			continue // these files do not support binary patching.
		}

		count := bytes.Count(data, magicString)
		if count == 0 {
			t.Errorf("%s binary does not contain magic string", file)
		} else if count > 1 {
			t.Errorf("%s binary contains magic string more than once", file)
		}
	}
}

func TestPatch(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		patch   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "no patch",
			content: []byte("foo\n" + string(magicString)),
			patch:   []byte(""),
			want:    []byte("foo\n" + string(magicString)),
		},
		{
			name:    "nil patch",
			content: []byte("foo"),
			patch:   nil,
			want:    []byte("foo"),
		},
		{
			name:    "no magic string",
			content: []byte("foo"),
			patch:   []byte("bar"),
			want:    []byte("foo"),
		},
		{
			name:    "patch too long",
			content: []byte("foo\n" + string(magicString)),
			patch:   make([]byte, 1024),
			wantErr: true,
		},
		{
			name:    "patch",
			content: []byte("foo\n" + string(magicString)),
			patch:   []byte("baz"),
			want:    []byte("foo\nbaz" + string(magicStringPadding[3:])),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Patch(tt.content, tt.patch)
			if (err != nil) != tt.wantErr {
				t.Errorf("Patch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Patch() = %v, want %v", got, tt.want)
			}
		})
	}
}
