package render

import (
	"text/template"
	"time"
)

const leftDelim = "{{"

// config is the resolved set of rendering options.
type config struct {
	funcs          template.FuncMap
	missingKeyErr  bool
	selfKey        string
	skip           func(path string) bool
	maxOutputBytes int
	maxTotalBytes  int
	outputDeadline time.Duration
}

func defaults() config {
	return config{
		missingKeyErr:  true,
		selfKey:        "self",
		maxOutputBytes: 1 << 20, // 1 MiB per field
		maxTotalBytes:  8 << 20, // 8 MiB per Value call
		outputDeadline: 2 * time.Second,
	}
}

// Option configures a Value call.
type Option func(*config)

// WithFuncs registers template helper functions available to every templated
// field.
func WithFuncs(funcs template.FuncMap) Option {
	return func(c *config) { c.funcs = funcs }
}

// WithMissingKeyError controls whether referencing a missing map key is an
// error (true, the default) or renders as "<no value>" (false). The index
// function is unaffected and returns no value for a missing key.
func WithMissingKeyError(b bool) Option {
	return func(c *config) { c.missingKeyErr = b }
}

// WithSelfKey sets the root key under which the document is exposed to its own
// templates. The default is "self".
func WithSelfKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.selfKey = key
		}
	}
}

// WithSkip excludes string values from rendering when skip reports true for
// their paths. Skipped values stay readable through the self key, unrendered.
// Paths look like "spec.interfaces[0].dhcp.mac"; map keys that do not match
// path identifiers are quoted, for example `["a.b"]`.
func WithSkip(skip func(path string) bool) Option {
	return func(c *config) { c.skip = skip }
}

// WithMaxOutputBytes caps the rendered size of any single field. Exceeding it
// fails with an error that is ErrOutputTooLarge. The default is 1 MiB; 0
// disables the cap.
func WithMaxOutputBytes(n int) Option {
	return func(c *config) { c.maxOutputBytes = max(n, 0) }
}

// WithMaxTotalBytes caps the combined rendered size of all fields in one Value
// call. Exceeding it fails with an error that is ErrTotalOutputTooLarge. The
// default is 8 MiB; 0 disables the cap.
func WithMaxTotalBytes(n int) Option {
	return func(c *config) { c.maxTotalBytes = max(n, 0) }
}

// WithOutputDeadline sets a best-effort deadline checked whenever a field writes
// output. It does not interrupt template functions or execution that produces no
// output. The default is 2s; 0 disables the deadline.
func WithOutputDeadline(d time.Duration) Option {
	return func(c *config) { c.outputDeadline = max(d, 0) }
}
