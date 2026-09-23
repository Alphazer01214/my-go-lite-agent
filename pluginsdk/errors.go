package pluginsdk

import (
	"errors"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// Re-export wire error codes so plugins only need this package for constants.
const (
	CodeMethodNotFound = protocol.CodeMethodNotFound
	CodeHandlerError   = protocol.CodeHandlerError
	CodeBadPayload     = protocol.CodeBadPayload
	CodeBadArguments   = protocol.CodeBadArguments
	CodeServerClosed   = protocol.CodeServerClosed
	HostCapability     = protocol.HostCapability
)

// codedError is an error carrying a stable wire error_code.
type codedError struct {
	code string
	msg  string
}

func (e *codedError) Error() string { return e.msg }

// ErrCode builds an error with a stable error_code for handler returns and Call.
// Dispatch writes code into res.error_code; empty code becomes handler_error.
func ErrCode(code, msg string) error {
	return &codedError{code: code, msg: msg}
}

// Code returns the error_code carried by err, or "" for a plain error.
// Unwraps fmt.Errorf("%w", ErrCode(...)) chains.
func Code(err error) string {
	if err == nil {
		return ""
	}
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return ""
}
