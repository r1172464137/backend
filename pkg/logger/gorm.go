package logger

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormLogger is GORM's logger for the database connections. It writes only
// failed and slow queries, and of a query only its operation and row count:
// the statement text can carry the values of personal columns.
type GormLogger struct {
	// SlowThreshold is the duration from which a query is logged as slow;
	// zero means one second.
	SlowThreshold time.Duration
}

// TAG prefixes the messages GormLogger writes.
const TAG = "[GORM]"

// LogMode keeps the process log level: GORM's level is ignored, and the
// process level in force is logged instead.
func (l *GormLogger) LogMode(logger.LogLevel) logger.Interface {
	var sysLevel string
	switch logLevel {
	case DebugLevel:
		sysLevel = "debug"
	case InfoLevel:
		sysLevel = "info"
	case SevereLevel:
		sysLevel = "severe"
	case disableLevel:
		sysLevel = "disable"
	default:
		sysLevel = "unknown"
	}
	Infof("%s System Log Level is %s", TAG, sysLevel)
	return l
}

// Info, Warn and Error log that GORM reported something, without the
// message, which can carry query values.
func (l *GormLogger) Info(ctx context.Context, str string, args ...any) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Info", TAG)
}

func (l *GormLogger) Warn(ctx context.Context, str string, args ...any) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Warn", TAG)
}

func (l *GormLogger) Error(ctx context.Context, str string, args ...any) {
	WithContext(ctx).WithCallerSkip(2).Errorf("%s Error", TAG)
}

// Trace logs a query that failed or took SlowThreshold or longer; a missed
// lookup (gorm.ErrRecordNotFound) is logged only when it was slow.
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	duration := time.Since(begin)
	threshold := l.SlowThreshold
	if threshold <= 0 {
		threshold = time.Second
	}

	// The expanded SQL callback is comparatively expensive and may contain
	// sensitive values. Do not invoke it for the overwhelmingly common fast,
	// successful query path.
	if err == nil && duration < threshold {
		return
	}
	// Record-not-found is normal control flow for cache probes and optional
	// records. It is only operationally interesting when the lookup itself was
	// slow.
	if errors.Is(err, gorm.ErrRecordNotFound) && duration < threshold {
		return
	}

	sql, rowsAffected := fc()
	fields := []LogField{
		{
			Key:   "operation",
			Value: sqlOperation(sql),
		},
		{
			Key:   "rows",
			Value: rowsAffected,
		},
	}
	if err != nil {
		fields = append(fields, LogField{
			Key:   "error",
			Value: err.Error(),
		})
		// A missed lookup is an expected outcome the caller handles (inbox
		// dedup probes, lazily-created rows, existence checks) — logging it
		// as an error drowns out real failures.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Sloww(TAG+" Slow Query", fields...)
		} else {
			WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Errorw(TAG, fields...)
		}
	} else {
		WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Sloww(TAG+" Slow Query", fields...)
	}
}

func sqlOperation(query string) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "UNKNOWN"
	}
	operation := strings.ToUpper(parts[0])
	switch operation {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "TRUNCATE":
		return operation
	default:
		return "OTHER"
	}
}
