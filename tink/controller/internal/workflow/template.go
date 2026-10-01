package workflow

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/tinkerbell/tinkerbell/pkg/template/funcmap"
)

// maxRenderBytes caps rendered template output to guard against expansion-based DoS.
const maxRenderBytes = 256 * 1024

// renderTemplate parses and executes a Go template with the hermetic function
// map, erroring on missing keys and capping output at maxRenderBytes.
func renderTemplate(name, tmplStr string, data interface{}) ([]byte, error) {
	t, err := template.New(name).
		Option("missingkey=error").
		Funcs(funcmap.New()).
		Parse(tmplStr)
	if err != nil {
		return nil, err
	}

	w := &limitedWriter{limit: maxRenderBytes}
	if err := t.Execute(w, data); err != nil {
		return nil, err
	}

	return w.buf.Bytes(), nil
}

// limitedWriter buffers written bytes and errors once limit is exceeded.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.limit {
		return 0, fmt.Errorf("rendered template exceeds %d bytes", w.limit)
	}
	return w.buf.Write(p)
}
