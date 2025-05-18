package delimitedlist

import (
	"strings"
)

// Value implements a user defined delimited list flag value.
type Value struct {
	target    *[]string
	delimiter rune
}

// New creates a new user defined delimited list value.
func New(target *[]string, d rune) *Value {
	return &Value{target: target, delimiter: d}
}

// FromEnv implements ff/v4's environmentally-sourced flag values.
func (v *Value) FromEnv(s string) error {
	return v.Set(s)
}

// FromFile implements ff/v4's file-sourced flag values.
func (v *Value) FromFile(s string) error {
	return v.Set(s)
}

// Set implements the flag.Value interface.
func (v *Value) Set(s string) error {
	values := strings.FieldsFunc(s, func(r rune) bool {
		return r == v.delimiter
	})
	*v.target = append(*v.target, values...)
	return nil
}

// String implements the flag.Value interface.
func (v *Value) String() string {
	if v.target == nil {
		return ""
	}
	return strings.Join(*v.target, string(v.delimiter))
}
