package logger

import (
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const callerDepth = 4

var (
	timeFormat        = "2006-01-02T15:04:05.000Z07:00"
	encoding   uint32 = jsonEncodingType
	// maxContentLength is used to truncate the log content, 0 for not truncating.
	maxContentLength uint32
	// use uint32 for atomic operations
	logLevel  uint32
	options   logOptions
	writer    = new(atomicWriter)
	setupOnce sync.Once
)

type (
	// LogField is a key-value pair that will be added to the log entry.
	LogField struct {
		Key               string
		Value             any
		allowRiskMetadata bool
	}

	// logOption customizes the file output.
	logOption func(options *logOptions)

	logEntry map[string]any

	logOptions struct {
		gzipEnabled           bool
		logStackCooldownMills int
		keepDays              int
		maxBackups            int
		maxSize               int
		rotationRule          string
	}
)

// Close flushes and closes the logging output; entries logged afterwards go
// to the console.
func Close() error {
	if w := writer.Swap(nil); w != nil {
		return w.Close()
	}

	return nil
}

// Debug writes v into access log.
func Debug(v ...any) {
	if shallLog(DebugLevel) {
		msg, fields := splitLogArgs(v)
		writeDebug(msg, fields...)
	}
}

// Disable disables the logging.
func Disable() {
	atomic.StoreUint32(&logLevel, disableLevel)
	writer.Store(nopWriter{})
}

// Error writes v into error log.
func Error(v ...any) {
	if shallLog(ErrorLevel) {
		msg, fields := splitLogArgs(v)
		writeError(msg, fields...)
	}
}

// Errorf writes v with format into error log.
func Errorf(format string, v ...any) {
	if shallLog(ErrorLevel) {
		writeError(fmt.Errorf(format, v...).Error())
	}
}

// errorStack writes v along with call stack into error log.
func errorStack(v ...any) {
	if shallLog(ErrorLevel) {
		// there is newline in stack string
		writeStack(redactText(fmt.Sprint(v...)))
	}
}

// Errorw writes msg along with fields into the error log.
func Errorw(msg string, fields ...LogField) {
	if shallLog(ErrorLevel) {
		writeError(msg, fields...)
	}
}

// Field returns a LogField for the given key and value.
func Field(key string, value any) LogField {
	var field LogField
	switch val := value.(type) {
	case error:
		field = LogField{Key: key, Value: encodeError(val)}
	case []error:
		var errs []string
		for _, err := range val {
			errs = append(errs, encodeError(err))
		}
		field = LogField{Key: key, Value: errs}
	case time.Duration:
		field = LogField{Key: key, Value: fmt.Sprint(val)}
	case []time.Duration:
		var durs []string
		for _, dur := range val {
			durs = append(durs, fmt.Sprint(dur))
		}
		field = LogField{Key: key, Value: durs}
	case []time.Time:
		var times []string
		for _, t := range val {
			times = append(times, fmt.Sprint(t))
		}
		field = LogField{Key: key, Value: times}
	case fmt.Stringer:
		field = LogField{Key: key, Value: encodeStringer(val)}
	case []fmt.Stringer:
		var strs []string
		for _, str := range val {
			strs = append(strs, encodeStringer(str))
		}
		field = LogField{Key: key, Value: strs}
	default:
		field = LogField{Key: key, Value: val}
	}
	return redactField(field)
}

// Info writes v into access log.
func Info(v ...any) {
	if shallLog(InfoLevel) {
		msg, fields := splitLogArgs(v)
		writeInfo(msg, fields...)
	}
}

// Infof writes v with format into access log.
func Infof(format string, v ...any) {
	if shallLog(InfoLevel) {
		writeInfo(fmt.Sprintf(format, v...))
	}
}

// Infow writes msg along with fields into the access log.
func Infow(msg string, fields ...LogField) {
	if shallLog(InfoLevel) {
		writeInfo(msg, fields...)
	}
}

// Reset removes the output and returns it, so that a test can install its
// own and restore this one with SetWriter; until an output is set, entries
// go to a new console output.
func Reset() Writer {
	return writer.Swap(nil)
}

// setLevel sets the logging level. It can be used to suppress some logs.
func setLevel(level uint32) {
	atomic.StoreUint32(&logLevel, level)
}

// SetWriter sets the logging writer. It can be used to customize the logging.
func SetWriter(w Writer) {
	if atomic.LoadUint32(&logLevel) != disableLevel {
		writer.Store(w)
	}
}

// SetUp configures the logger from c. Only the first call takes effect;
// later calls change nothing and return nil, so the process keeps the
// output it logged to first.
func SetUp(c LogConf) (err error) {
	setupOnce.Do(func() {
		setupLogLevel(c)

		if len(c.TimeFormat) > 0 {
			timeFormat = c.TimeFormat
		}

		if len(c.FileTimeFormat) > 0 {
			fileTimeFormat = c.FileTimeFormat
		}

		atomic.StoreUint32(&maxContentLength, c.MaxContentLength)
		switch c.Encoding {
		case plainEncoding:
			atomic.StoreUint32(&encoding, plainEncodingType)
		default:
			atomic.StoreUint32(&encoding, jsonEncodingType)
		}

		switch c.Mode {
		case fileMode:
			err = setupWithFiles(c)
		case volumeMode:
			err = setupWithVolume(c)
		default:
			setupWithConsole()
		}
	})

	return
}

// withCooldownMillis customizes logging on writing call stack interval.
func withCooldownMillis(millis int) logOption {
	return func(opts *logOptions) {
		opts.logStackCooldownMills = millis
	}
}

// withKeepDays customizes logging to keep logs with days.
func withKeepDays(days int) logOption {
	return func(opts *logOptions) {
		opts.keepDays = days
	}
}

// withGzip customizes logging to automatically gzip the log files.
func withGzip() logOption {
	return func(opts *logOptions) {
		opts.gzipEnabled = true
	}
}

// withMaxBackups customizes how many log files backups will be kept.
func withMaxBackups(count int) logOption {
	return func(opts *logOptions) {
		opts.maxBackups = count
	}
}

// withMaxSize customizes how much space the writing log file can take up.
func withMaxSize(size int) logOption {
	return func(opts *logOptions) {
		opts.maxSize = size
	}
}

// withRotation customizes which log rotation rule to use.
func withRotation(r string) logOption {
	return func(opts *logOptions) {
		opts.rotationRule = r
	}
}

func addCaller(fields ...LogField) []LogField {
	return append(fields, Field(callerKey, getCaller(callerDepth)))
}

func createOutput(path string) (io.WriteCloser, error) {
	if len(path) == 0 {
		return nil, ErrLogPathNotSet
	}

	var rule RotateRule
	switch options.rotationRule {
	case sizeRotationRule:
		rule = newSizeLimitRotateRule(path, options.keepDays, options.maxSize,
			options.maxBackups, options.gzipEnabled)
	default:
		rule = defaultRotateRule(path, options.keepDays, options.gzipEnabled)
	}

	return newRotateLogger(path, rule, options.gzipEnabled)
}

func encodeError(err error) (ret string) {
	return encodeWithRecover(err, func() string {
		return err.Error()
	})
}

func encodeStringer(v fmt.Stringer) (ret string) {
	return encodeWithRecover(v, func() string {
		return v.String()
	})
}

func encodeWithRecover(arg any, fn func() string) (ret string) {
	defer func() {
		if err := recover(); err != nil {
			if v := reflect.ValueOf(arg); v.Kind() == reflect.Pointer && v.IsNil() {
				ret = nilAngleString
			} else {
				ret = fmt.Sprintf("panic: %v", err)
			}
		}
	}()

	return fn()
}

func getWriter() Writer {
	w := writer.Load()
	if w == nil {
		w = writer.StoreIfNil(newConsoleWriter())
	}

	return w
}

func handleOptions(opts []logOption) {
	for _, opt := range opts {
		opt(&options)
	}
}

func setupLogLevel(c LogConf) {
	switch c.Level {
	case levelDebug:
		setLevel(DebugLevel)
	case levelInfo:
		setLevel(InfoLevel)
	case levelError:
		setLevel(ErrorLevel)
	case levelSevere:
		setLevel(SevereLevel)
	}
}

func setupWithConsole() {
	SetWriter(newConsoleWriter())
}

func setupWithFiles(c LogConf) error {
	w, err := newFileWriter(c)
	if err != nil {
		return err
	}

	SetWriter(w)
	return nil
}

func setupWithVolume(c LogConf) error {
	if len(c.ServiceName) == 0 {
		return ErrLogServiceNameNotSet
	}
	hostname, _ := os.Hostname()
	c.Path = path.Join(c.Path, c.ServiceName, hostname)
	return setupWithFiles(c)
}

func shallLog(level uint32) bool {
	return atomic.LoadUint32(&logLevel) <= level
}

// writeDebug writes v into debug log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeDebug(val any, fields ...LogField) {
	getWriter().Debug(redactValue(val), redactFields(addCaller(fields...))...)
}

// writeError writes v into the error log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeError(val any, fields ...LogField) {
	getWriter().Error(redactValue(val), redactFields(addCaller(fields...))...)
}

// writeInfo writes v into info log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeInfo(val any, fields ...LogField) {
	getWriter().Info(redactValue(val), redactFields(addCaller(fields...))...)
}

// writeStack writes v into stack log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeStack(msg string) {
	getWriter().Stack(fmt.Sprintf("%s\n%s", redactText(msg), string(debug.Stack())))
}
