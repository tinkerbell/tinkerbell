package httperror

import (
	"errors"
	"fmt"
)

// E is the error struct that can be used with errors.As and errors.Is. Consumers of E are
// typically aware they're serving HTTP and consequently have sufficient context to set a
// StatusCode.
type E struct {
	// StatusCode is the HTTP status code this error is best represented by.
	StatusCode int
	E          error
}

// New constructs a new E with code and error message.
func New(code int, msg string) error {
	return &E{
		StatusCode: code,
		E:          errors.New(msg),
	}
}

// Newf constructs a new E with code and error message formatted with fmt.Sprintf.
func Newf(code int, format string, args ...any) error {
	return New(code, fmt.Sprintf(format, args...))
}

// Wrap wraps an error with in an E instance.
func Wrap(code int, err error) error {
	return &E{
		StatusCode: code,
		E:          err,
	}
}

// Error satisfies the error interface.
func (e *E) Error() string {
	return e.E.Error()
}
