package binary

import (
	"bytes"
	"testing"
)

func TestBinariesContainMagicString(t *testing.T) {
	for file, data := range Files {
		if file == "undionly.kpxe" {
			continue // undionly.kpxe does not support binary patching
		}

		count := bytes.Count(data, MagicString)
		if count == 0 {
			t.Errorf("%s binary does not contain magic string", file)
		} else if count > 1 {
			t.Errorf("%s binary contains magic string more than once", file)
		}
	}
}
