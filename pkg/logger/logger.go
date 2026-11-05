// Package logger is the process log. Entries are leveled, carry structured
// fields and are written as JSON or plain text to the console or to rotated
// files (access, error and slow), with credentials and personal data
// redacted before they are written. Code on a request or task path logs
// through WithContext(ctx), so each entry carries the trace and span of the
// request that caused it; GormLogger reports failed and slow SQL through the
// same output.
package logger

import (
	"time"
)

// A Logger writes entries that carry the trace and span of the context it
// was made for (see WithContext), the fields stored in that context and the
// caller's position.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(...any)
	// Debugf logs a message at debug level.
	Debugf(string, ...any)
	// Debugw logs a message at debug level.
	Debugw(string, ...LogField)
	// Error logs a message at error level.
	Error(...any)
	// Errorf logs a message at error level.
	Errorf(string, ...any)
	// Errorw logs a message at error level.
	Errorw(string, ...LogField)
	// Info logs a message at info level.
	Info(...any)
	// Infof logs a message at info level.
	Infof(string, ...any)
	// Infow logs a message at info level.
	Infow(string, ...LogField)
	// Sloww logs a message at slow level.
	Sloww(string, ...LogField)
	// WithCallerSkip returns a new logger with the given caller skip.
	WithCallerSkip(skip int) Logger
	// WithDuration returns a new logger with the given duration.
	WithDuration(d time.Duration) Logger
}
