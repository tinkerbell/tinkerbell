package spacelist

import (
	"strings"
)

// Value implements a space-delimited list flag value.
type Value struct {
	target *[]string
}

// New creates a new space-delimited list value.
func New(target *[]string) *Value {
	return &Value{target: target}
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
	values := strings.Fields(s)
	*v.target = append(*v.target, values...)
	return nil
}

// String implements the flag.Value interface.
func (v *Value) String() string {
	if v.target == nil {
		return ""
	}
	return strings.Join(*v.target, " ")
}
