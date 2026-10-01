package render

import (
	"errors"
	"fmt"
	"strings"
)

// ErrReferenceCycle is returned by Value when self-references between
// fields form a cycle, so no evaluation order exists. Use errors.As with
// *CycleError to inspect the fields involved.
var ErrReferenceCycle = errors.New("render: reference cycle")

// ErrOutputTooLarge is returned, wrapped in a *FieldError, when a field's
// rendered output exceeds the maximum size (see WithMaxOutputBytes).
var ErrOutputTooLarge = errors.New("render: output too large")

// ErrTotalOutputTooLarge is returned, wrapped in a *FieldError, when the
// combined rendered output of all fields exceeds the maximum size (see
// WithMaxTotalBytes).
var ErrTotalOutputTooLarge = errors.New("render: total output too large")

// ErrOutputDeadline is returned, wrapped in a *FieldError, when output is
// written after the deadline set by WithOutputDeadline.
var ErrOutputDeadline = errors.New("render: output deadline exceeded")

// CycleError reports a reference cycle and the field paths involved, in the
// order they reference one another.
type CycleError struct {
	// Fields is the cycle as a path, e.g. ["a", "b", "a"].
	Fields []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("render: reference cycle: %s", strings.Join(e.Fields, " -> "))
}

// Is reports whether the target is ErrReferenceCycle.
func (e *CycleError) Is(target error) bool { return target == ErrReferenceCycle }

// FieldError annotates a template parse or execution failure with the path of
// the field that caused it, for example "spec.instance.hostname" or
// "spec.interfaces[0].dhcp.hostname".
type FieldError struct {
	// Path is the dotted/indexed location of the offending field.
	Path string
	// Err is the underlying parse or execution error.
	Err error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("render: field %q: %v", e.Path, e.Err)
}

func (e *FieldError) Unwrap() error { return e.Err }
