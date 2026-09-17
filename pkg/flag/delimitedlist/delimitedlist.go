package delimitedlist

import (
	"fmt"
	"slices"
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

// ParsedValue implements a delimited list flag whose items are parsed into T.
// Each successful Set replaces the previous list.
type ParsedValue[T any] struct {
	target    *[]T
	initial   []T
	delimiter rune
	parse     func(string) (T, error)
}

// NewParsed creates a parsed delimited list value.
func NewParsed[T any](target *[]T, d rune, parse func(string) (T, error)) *ParsedValue[T] {
	var initial []T
	if target != nil {
		initial = slices.Clone(*target)
	}
	return &ParsedValue[T]{target: target, initial: initial, delimiter: d, parse: parse}
}

// Set parses a trimmed delimited list and replaces the target atomically.
func (v *ParsedValue[T]) Set(s string) error {
	if v == nil || v.target == nil {
		return fmt.Errorf("list target is nil")
	}
	if v.parse == nil {
		return fmt.Errorf("list parser is nil")
	}

	values := strings.FieldsFunc(s, func(r rune) bool {
		return r == v.delimiter
	})
	parsed := make([]T, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		item, err := v.parse(value)
		if err != nil {
			return err
		}
		parsed = append(parsed, item)
	}

	*v.target = parsed
	return nil
}

// Reset restores the target list to its initial value.
func (v *ParsedValue[T]) Reset() error {
	if v == nil || v.target == nil {
		return fmt.Errorf("list target is nil")
	}
	*v.target = slices.Clone(v.initial)
	return nil
}

// String returns the delimited string representation of the list.
func (v *ParsedValue[T]) String() string {
	if v == nil || v.target == nil {
		return ""
	}
	values := make([]string, 0, len(*v.target))
	for _, value := range *v.target {
		values = append(values, fmt.Sprint(value))
	}
	return strings.Join(values, string(v.delimiter))
}
