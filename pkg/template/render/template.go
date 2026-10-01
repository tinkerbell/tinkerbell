package render

import (
	"bytes"
	"text/template"
	"time"
)

// buildTemplate parses tmpl with the configured functions and missing-key
// behavior.
func buildTemplate(tmpl string, cfg *config) (*template.Template, error) {
	t := template.New("render")
	if cfg.funcs != nil {
		t = t.Funcs(cfg.funcs)
	}
	if cfg.missingKeyErr {
		t = t.Option("missingkey=error")
	}
	return t.Parse(tmpl)
}

// limitWriter caps output size and checks the output deadline on each write.
type limitWriter struct {
	buf      bytes.Buffer
	max      int
	total    *int // bytes left for the whole Value call; nil is unlimited
	deadline time.Time
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if !w.deadline.IsZero() && time.Now().After(w.deadline) {
		return 0, ErrOutputDeadline
	}
	if w.max > 0 && w.buf.Len()+len(p) > w.max {
		n, _ := w.buf.Write(p[:w.max-w.buf.Len()])
		return n, ErrOutputTooLarge
	}
	if w.total != nil {
		if len(p) > *w.total {
			return 0, ErrTotalOutputTooLarge
		}
		*w.total -= len(p)
	}
	return w.buf.Write(p)
}

// execTemplate executes t against root under the configured size caps and
// output deadline, charging its output to total.
func execTemplate(t *template.Template, root map[string]any, total *int, cfg *config) (string, error) {
	w := &limitWriter{max: cfg.maxOutputBytes, total: total}
	if cfg.outputDeadline > 0 {
		w.deadline = time.Now().Add(cfg.outputDeadline)
	}
	if err := t.Execute(w, root); err != nil {
		return "", err
	}
	return w.buf.String(), nil
}
