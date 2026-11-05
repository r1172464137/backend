package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"
	"runtime/debug"
	"sync"
	"sync/atomic"

	fatihcolor "github.com/fatih/color"
)

type (
	// A Writer is the output the log entries go to; SetWriter installs one.
	// Each method writes one entry of its level, with fields next to v.
	Writer interface {
		Close() error
		Debug(v any, fields ...LogField)
		Error(v any, fields ...LogField)
		Info(v any, fields ...LogField)
		Slow(v any, fields ...LogField)
		// Stack writes v, a message followed by its call stack, at error
		// level.
		Stack(v any)
	}

	atomicWriter struct {
		writer Writer
		lock   sync.RWMutex
	}

	concreteWriter struct {
		infoLog  io.WriteCloser
		errorLog io.WriteCloser
		slowLog  io.WriteCloser
		stackLog io.Writer
	}
)

// NewWriter returns a Writer that writes the entries of every level to w.
func NewWriter(w io.Writer) Writer {
	lw := newLogWriter(log.New(w, "", flags))

	return &concreteWriter{
		infoLog:  lw,
		errorLog: lw,
		slowLog:  lw,
		stackLog: lw,
	}
}

func (w *atomicWriter) Load() Writer {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.writer
}

func (w *atomicWriter) Store(v Writer) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.writer = v
}

func (w *atomicWriter) StoreIfNil(v Writer) Writer {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.writer == nil {
		w.writer = v
	}

	return w.writer
}

func (w *atomicWriter) Swap(v Writer) Writer {
	w.lock.Lock()
	defer w.lock.Unlock()
	old := w.writer
	w.writer = v
	return old
}

func newConsoleWriter() Writer {
	outLog := newLogWriter(log.New(fatihcolor.Output, "", flags))
	errLog := newLogWriter(log.New(fatihcolor.Error, "", flags))
	return &concreteWriter{
		infoLog:  outLog,
		errorLog: errLog,
		slowLog:  errLog,
		stackLog: newLessWriter(errLog, options.logStackCooldownMills),
	}
}

func newFileWriter(c LogConf) (Writer, error) {
	var err error
	var opts []logOption
	var infoLog io.WriteCloser
	var errorLog io.WriteCloser
	var slowLog io.WriteCloser
	var stackLog io.Writer

	if len(c.Path) == 0 {
		return nil, ErrLogPathNotSet
	}

	opts = append(opts, withCooldownMillis(c.StackCooldownMillis))
	if c.Compress {
		opts = append(opts, withGzip())
	}
	if c.KeepDays > 0 {
		opts = append(opts, withKeepDays(c.KeepDays))
	}
	if c.MaxBackups > 0 {
		opts = append(opts, withMaxBackups(c.MaxBackups))
	}
	if c.MaxSize > 0 {
		opts = append(opts, withMaxSize(c.MaxSize))
	}

	opts = append(opts, withRotation(c.Rotation))

	accessFile := path.Join(c.Path, accessFilename)
	errorFile := path.Join(c.Path, errorFilename)
	slowFile := path.Join(c.Path, slowFilename)

	handleOptions(opts)
	setupLogLevel(c)

	if infoLog, err = createOutput(accessFile); err != nil {
		return nil, err
	}

	if errorLog, err = createOutput(errorFile); err != nil {
		return nil, err
	}

	if slowLog, err = createOutput(slowFile); err != nil {
		return nil, err
	}

	stackLog = newLessWriter(errorLog, options.logStackCooldownMills)

	return &concreteWriter{
		infoLog:  infoLog,
		errorLog: errorLog,
		slowLog:  slowLog,
		stackLog: stackLog,
	}, nil
}

func (w *concreteWriter) Close() error {
	if err := w.infoLog.Close(); err != nil {
		return err
	}

	if err := w.errorLog.Close(); err != nil {
		return err
	}

	return w.slowLog.Close()
}

func (w *concreteWriter) Debug(v any, fields ...LogField) {
	output(w.infoLog, levelDebug, v, fields...)
}

func (w *concreteWriter) Error(v any, fields ...LogField) {
	output(w.errorLog, levelError, v, fields...)
}

func (w *concreteWriter) Info(v any, fields ...LogField) {
	output(w.infoLog, levelInfo, v, fields...)
}

func (w *concreteWriter) Slow(v any, fields ...LogField) {
	output(w.slowLog, levelSlow, v, fields...)
}

func (w *concreteWriter) Stack(v any) {
	output(w.stackLog, levelError, v)
}

type nopWriter struct{}

func (n nopWriter) Close() error {
	return nil
}

func (n nopWriter) Debug(_ any, _ ...LogField) {
}

func (n nopWriter) Error(_ any, _ ...LogField) {
}

func (n nopWriter) Info(_ any, _ ...LogField) {
}

func (n nopWriter) Slow(_ any, _ ...LogField) {
}

func (n nopWriter) Stack(_ any) {
}

func buildPlainFields(fields logEntry) []string {
	orderedKeys := []string{"duration", "caller"}
	items := make([]string, 0, len(fields))
	for _, key := range orderedKeys {
		// append the keys that have been set
		if value, exists := fields[key]; exists {
			items = append(items, fmt.Sprintf("%s=%+v", key, value))
		}
	}
	for key, value := range fields {
		if !contains(orderedKeys, key) { // skip the keys that have been appended
			items = append(items, fmt.Sprintf("%s=%+v", key, value))
		}
	}
	return items
}

func marshalJson(t any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	// go 1.5+ will append a newline to the end of the json string
	// https://github.com/golang/go/issues/13520
	if l := buf.Len(); l > 0 && buf.Bytes()[l-1] == '\n' {
		buf.Truncate(l - 1)
	}

	return buf.Bytes(), err
}

func output(writer io.Writer, level string, val any, fields ...LogField) {
	val = redactValue(val)
	fields = redactFields(fields)
	maxLen := atomic.LoadUint32(&maxContentLength)
	var truncated bool
	val, truncated = limitValue(val, maxLen, 0)
	for i := range fields {
		var cut bool
		fields[i].Value, cut = limitValue(fields[i].Value, maxLen, 0)
		truncated = truncated || cut
	}
	if truncated {
		fields = append(fields, truncatedField)
	}
	// +3 for timestamp, level and content
	entry := make(logEntry, len(fields)+3)
	for _, field := range fields {
		entry[field.Key] = field.Value
	}

	switch atomic.LoadUint32(&encoding) {
	case plainEncodingType:
		plainFields := buildPlainFields(entry)
		writePlainAny(writer, level, val, plainFields...)
	default:
		entry[timestampKey] = getTimestamp()
		entry[levelKey] = level
		entry[contentKey] = val
		writeJson(writer, entry)
	}
}

func wrapLevelWithColor(level string) string {
	var colour fatihcolor.Attribute
	switch level {
	case levelError:
		colour = fatihcolor.FgRed
	case levelInfo:
		colour = fatihcolor.FgBlue
	case levelSlow:
		colour = fatihcolor.FgYellow
	case levelDebug:
		colour = fatihcolor.FgYellow
	default:
		return level
	}

	return fatihcolor.New(colour, fatihcolor.Bold).Sprint(" " + level + " ")
}

func writeJson(writer io.Writer, info any) {
	if content, err := marshalJson(info); err != nil {
		log.Printf("err: %s\n\n%s", err.Error(), debug.Stack())
	} else if writer == nil {
		log.Println(string(content))
	} else {
		if _, err := writer.Write(append(content, '\n')); err != nil {
			log.Println(err.Error())
		}
	}
}

func writePlainAny(writer io.Writer, level string, val any, fields ...string) {
	level = wrapLevelWithColor(level)

	switch v := val.(type) {
	case string:
		writePlainText(writer, level, v, fields...)
	case error:
		writePlainText(writer, level, v.Error(), fields...)
	case fmt.Stringer:
		writePlainText(writer, level, v.String(), fields...)
	default:
		writePlainValue(writer, level, v, fields...)
	}
}

func writePlainText(writer io.Writer, level, msg string, fields ...string) {
	var buf bytes.Buffer
	buf.WriteString(getTimestamp())
	buf.WriteByte(plainEncodingSep)
	buf.WriteString(level)
	buf.WriteByte(plainEncodingSep)
	buf.WriteString(msg)
	for _, item := range fields {
		buf.WriteByte(plainEncodingSep)
		buf.WriteString(item)
	}
	buf.WriteByte('\n')
	if writer == nil {
		log.Println(buf.String())
		return
	}

	if _, err := writer.Write(buf.Bytes()); err != nil {
		log.Println(err.Error())
	}
}

func writePlainValue(writer io.Writer, level string, val any, fields ...string) {
	var buf bytes.Buffer
	buf.WriteString(getTimestamp())
	buf.WriteByte(plainEncodingSep)
	buf.WriteString(level)
	buf.WriteByte(plainEncodingSep)
	if err := json.NewEncoder(&buf).Encode(val); err != nil {
		log.Printf("err: %s\n\n%s", err.Error(), debug.Stack())
		return
	}

	for _, item := range fields {
		buf.WriteByte(plainEncodingSep)
		buf.WriteString(item)
	}
	buf.WriteByte('\n')
	if writer == nil {
		log.Println(buf.String())
		return
	}

	if _, err := writer.Write(buf.Bytes()); err != nil {
		log.Println(err.Error())
	}
}
