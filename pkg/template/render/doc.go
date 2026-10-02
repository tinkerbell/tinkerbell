// Package render renders Go text/template expressions embedded in the string
// values of an already-decoded document, resolving them against the document
// itself (self-reference) and against caller-supplied data.
//
// Only string values containing "{{" are selected for rendering, and each result
// is a string. These replacements do not reshape the document, but caller-supplied
// helpers can mutate exposed maps and slices, including adding or removing fields.
// A literal "{{" is written as {{ "{{" }}.
//
// WithSkip excludes whole strings before parsing while keeping them readable.
// HasTemplates uses the same selection without executing or modifying the document.
//
// # Self-reference and evaluation order
//
// The document is exposed to its own templates under the self key (default
// "self"; see WithSelfKey), so a field can reference a sibling:
//
//	name: example
//	greeting: "hello {{ .self.name }}"
//
// Fields may reference other templated fields. Value builds a dependency graph
// from the self-references and evaluates fields in dependency order, so each
// field observes the rendered value of everything it depends on. Each field is
// evaluated exactly once. If the references form a cycle, Value returns an
// error that is ErrReferenceCycle.
//
// To decide render order, Value works out which parts of the document each
// template reads. A template that uses a value in full depends on that value
// and everything nested inside it, so all of those render first:
//
//	{{ .self.disks }}           printing it
//	{{ toJson .self.disks }}    passing it to a function or named template
//	{{ $d = .self.disks }}      reassigning an existing variable
//
// A variable declared with := is tracked through its later uses; for example,
// {{ $d := .self.disks }}{{ $d }} reads the disks value when $d is printed.
//
// A template that only tests a value depends on that value alone, not on
// what is nested inside it:
//
//	{{ if .self.disks }}  {{ with .self.disks }}  {{ range .self.disks }}
//	{{ len .self.disks }}  {{ not .self.disks }}
//	{{ if and .self.a .self.b }}   (and/or when used only for truthiness)
//
// Fields used inside an if, with or range body are tracked on their own, so
// a loop depends only on the fields it uses from each element:
//
//	disks:
//	  - name: sda
//	    label: "{{ range .self.disks }}{{ .name }} {{ end }}"
//
// Here label depends on every disk's name, but not on any disk's label. If
// the loop body used {{ .label }} instead, label would depend on itself and
// Value would report a cycle. The same applies to {{ toJson .self.disks }},
// which uses every field of every disk. The analysis errs on the side of
// caution, so it can report a cycle that would not actually occur when the
// template runs.
//
// Caller data is merged into the same root, so templates may also reference
// external values (for example {{ .references.net.spec.domain }}). Data values
// are never themselves templated.
//
// # Resource limits
//
// Each field is rendered under an output-size cap (WithMaxOutputBytes, default
// 1 MiB) and a best-effort output-write deadline (WithOutputDeadline, default
// 2s), and the combined output of all fields is capped too (WithMaxTotalBytes,
// default 8 MiB). A breach is reported as a *FieldError that is
// ErrOutputTooLarge, ErrTotalOutputTooLarge or ErrOutputDeadline. The deadline
// applies per field and is checked only when output is written; it cannot
// interrupt a blocked function or execution that produces no output.
//
// These limits are operational safeguards against mistakes, not a security
// boundary. They cap rendered output, not memory or CPU: a template can
// allocate or loop without writing, for example by growing a variable in a
// range. Only trusted authors should be able to supply templates.
package render
