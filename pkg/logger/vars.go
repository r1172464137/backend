package logger

import (
	"errors"
)

const (
	// DebugLevel logs everything
	DebugLevel uint32 = iota
	// InfoLevel does not include debugs
	InfoLevel
	// ErrorLevel includes errors, slows, stacks
	ErrorLevel
	// SevereLevel, the "severe" configuration level, is above every level
	// the server writes, so it silences the log.
	SevereLevel
	// disableLevel doesn't log any messages
	disableLevel = 0xff
)

const (
	jsonEncodingType = iota
	plainEncodingType
)

const (
	plainEncoding    = "plain"
	plainEncodingSep = '\t'
	sizeRotationRule = "size"

	accessFilename = "access.log"
	errorFilename  = "error.log"
	slowFilename   = "slow.log"

	fileMode   = "file"
	volumeMode = "volume"

	levelInfo   = "info"
	levelError  = "error"
	levelSevere = "severe"
	levelSlow   = "slow"
	levelDebug  = "debug"

	backupFileDelimiter = "-"
	nilAngleString      = "<nil>"
	flags               = 0x0
)

const (
	callerKey    = "caller"
	contentKey   = "content"
	durationKey  = "duration"
	levelKey     = "level"
	spanKey      = "span"
	timestampKey = "timestamp"
	traceKey     = "trace"
	truncatedKey = "truncated"
)

var (
	// ErrLogPathNotSet is an error that indicates the log path is not set.
	ErrLogPathNotSet = errors.New("log path must be set")
	// ErrLogServiceNameNotSet is an error that indicates that the service name is not set.
	ErrLogServiceNameNotSet = errors.New("log service name must be set")

	truncatedField = Field(truncatedKey, true)
)
