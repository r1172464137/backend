// Package xerr is the error model of the API. A CodeError carries one of
// the numeric codes of err_code.go and a message, which is all a client
// learns of a failure: a response carries the code and message of the first
// CodeError in the error chain, never the rest of the error text. Wrapf and
// Errorf attach a code while keeping the underlying error reachable for
// errors.Is and errors.As and for the log (Detail). Clients switch on the
// numeric values, so existing codes never change.
package xerr

import (
	"errors"
	"fmt"
	"strings"
)

// CodeError is an error with a client-facing code and message.
type CodeError struct {
	errCode uint32
	errMsg  string
	// cause is the failure the code was attached to by Wrapf, if any.
	cause error
}

// ErrNotModified reports that the client's cached copy is current: the
// handler answers 304 Not Modified instead of a body.
var ErrNotModified = errors.New("304 Not Modified")

// GetErrCode returns the error code displayed to the front end
func (e *CodeError) GetErrCode() uint32 {
	return e.errCode
}

// GetErrMsg returns the error message displayed to the front end
func (e *CodeError) GetErrMsg() string {
	return e.errMsg
}

// Error formats the code and its message, for logs.
func (e *CodeError) Error() string {
	return fmt.Sprintf("ErrCode:%d，ErrMsg:%s", e.errCode, e.errMsg)
}

// Unwrap returns the failure the code was attached to, so errors.Is and
// errors.As still reach it (a wrapped gorm.ErrRecordNotFound, for example).
func (e *CodeError) Unwrap() error {
	return e.cause
}

// NewErrCodeMsg returns an error with errCode and a message of its own,
// which the client receives in place of the code's usual message.
func NewErrCodeMsg(errCode uint32, errMsg string) *CodeError {
	return &CodeError{errCode: errCode, errMsg: errMsg}
}

// NewErrCode returns an error with errCode and the code's message.
func NewErrCode(errCode uint32) *CodeError {
	return &CodeError{errCode: errCode, errMsg: MapErrMsg(errCode)}
}

// NewErrMsg returns an ERROR with errMsg as the message the client receives.
func NewErrMsg(errMsg string) *CodeError {
	return &CodeError{errCode: ERROR, errMsg: errMsg}
}

// Wrapf reports err under code, prefixed with a formatted context message.
//
// A generic code (see IsGeneric) never replaces a code err already carries:
// the layer that produced err knew the specific failure, such as an exhausted
// coupon inside a transaction, and that is what the client must see. err stays
// reachable through errors.Is and errors.As either way. Wrapf returns nil when
// err is nil.
func Wrapf(err error, code uint32, format string, args ...any) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	if IsGeneric(code) {
		var coded *CodeError
		if errors.As(err, &coded) {
			return fmt.Errorf("%s: %w", msg, err)
		}
	}
	return fmt.Errorf("%s: %w", msg, &CodeError{errCode: code, errMsg: MapErrMsg(code), cause: err})
}

// Errorf returns an error carrying code with a formatted context message,
// for failures that have no underlying error to wrap.
func Errorf(code uint32, format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), NewErrCode(code))
}

// IsGeneric reports whether code only says that something failed (ERROR or
// a database failure) rather than what the client did wrong.
func IsGeneric(code uint32) bool {
	switch code {
	case ERROR, DatabaseQueryError, DatabaseUpdateError, DatabaseInsertError, DatabaseDeletedError:
		return true
	}
	return false
}

// CodeOf returns the code the client receives for err: the first code in its
// chain, or ERROR when it carries none.
func CodeOf(err error) uint32 {
	var coded *CodeError
	if errors.As(err, &coded) {
		return coded.GetErrCode()
	}
	return ERROR
}

// Detail describes err for a log line: its message, plus the cause of any
// code in the chain whose message does not already include that cause.
func Detail(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Each pass finds the next code in the chain and continues below it.
	for e := err; e != nil; {
		var coded *CodeError
		if !errors.As(e, &coded) || coded.cause == nil {
			break
		}
		if cause := coded.cause.Error(); !strings.Contains(msg, cause) {
			msg += " (cause: " + cause + ")"
		}
		e = coded.cause
	}
	return msg
}
