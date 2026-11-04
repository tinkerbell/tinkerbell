package main

import (
	"strings"

	"github.com/spf13/pflag"
)

// resettableSlice is a pflag slice value that can be reset to its default.
// ff resets a value it cannot reset itself by setting its default string, but
// once a pflag slice has been set, setting it again appends rather than
// replaces.
type resettableSlice struct {
	pflag.Value
	slice pflag.SliceValue
	def   []string
}

// newResettableSlice returns v with a Reset method if v is a pflag slice
// value, and v unchanged otherwise. The current value of v is its default.
func newResettableSlice(v pflag.Value) pflag.Value {
	s, ok := v.(pflag.SliceValue)
	if !ok {
		return v
	}
	return &resettableSlice{Value: v, slice: s, def: s.GetSlice()}
}

// Reset sets the value back to its default.
func (r *resettableSlice) Reset() error {
	return r.slice.Replace(r.def)
}

// GetPlaceholder returns the pflag type name in upper case, such as
// STRINGARRAY, which is the placeholder ff shows for the unwrapped value.
func (r *resettableSlice) GetPlaceholder() string {
	return strings.ToUpper(r.Type())
}
