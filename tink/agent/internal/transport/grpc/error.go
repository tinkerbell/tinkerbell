package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type kind int

const (
	kindNoAction kind = iota + 1
	kindConflict
	kindPermanent
)

// classifiedError tells the Agent how to retry a Tink Server response.
// See the Retry type in the agent package.
type classifiedError struct {
	kind kind
	err  error
}

func (e *classifiedError) Error() string   { return e.err.Error() }
func (e *classifiedError) Unwrap() error   { return e.err }
func (e *classifiedError) NoAction() bool  { return e.kind == kindNoAction }
func (e *classifiedError) Conflict() bool  { return e.kind == kindConflict }
func (e *classifiedError) Permanent() bool { return e.kind == kindPermanent }

// readError classifies a GetAction error.
func readError(err error) error {
	switch status.Code(err) { //nolint:exhaustive // every other code is retried with backoff
	case codes.NotFound, codes.FailedPrecondition:
		return &classifiedError{kind: kindNoAction, err: err}
	case codes.Aborted:
		return &classifiedError{kind: kindConflict, err: err}
	}
	return err
}

// writeError classifies a ReportActionStatus error.
func writeError(err error) error {
	switch status.Code(err) { //nolint:exhaustive // every other code is retried with backoff
	case codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition:
		return &classifiedError{kind: kindPermanent, err: err}
	case codes.Aborted:
		return &classifiedError{kind: kindConflict, err: err}
	}
	return err
}
