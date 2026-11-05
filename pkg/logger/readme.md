
## logger configurations

```go
type LogConf struct {
	ServiceName         string              
	Mode                string              
	Encoding            string              
	TimeFormat          string              
	Path                string              
	Level               string              
	Compress            bool                
	KeepDays            int                 
	StackCooldownMillis int                 
	MaxBackups          int                 
	MaxSize             int                 
	Rotation            string              
}
```

- `ServiceName`: set the service name, optional. on `volume` mode, the name is used to generate the log files. Within `rest/zrpc` services, the name will be set to the name of `rest` or `zrpc` automatically.
- `Mode`: the mode to output the logs, default is `console`.
  -  `console` mode writes the logs to `stdout/stderr`.
  - `file` mode writes the logs to the files specified by `Path`.
  - `volume` mode is used in docker, to write logs into mounted volumes.
- `Encoding`: indicates how to encode the logs, default is `json`.
  - `json` mode writes the logs in json format.
  - `plain` mode writes the logs with plain text, with terminal color enabled.
- `TimeFormat`: customize the time format, optional. Default is `2006-01-02T15:04:05.000Z07:00`.
- `Path`: set the log path, default to `logs`.
- `Level`: the logging level to filter logs. Default is `info`.
  - `info`, all logs are written.
  - `error`, `info` logs are suppressed.
  - `severe`, `info` and `error` logs are suppressed, only `severe` logs are written.
- `Compress`: whether or not to compress log files, only works with `file` mode.
- `KeepDays`: how many days that the log files are kept, after the given days, the outdated files will be deleted automatically. It has no effect on `console` mode.
- `StackCooldownMillis`: how many milliseconds to rewrite stacktrace again. It’s used to avoid stacktrace flooding.
- `MaxBackups`: represents how many backup log files will be kept. 0 means all files will be kept forever. Only take effect when `Rotation` is `size`. NOTE: the level of option `KeepDays` will be higher. Even though `MaxBackups` sets 0, log files will still be removed if the `KeepDays` limitation is reached.
- `MaxSize`: represents how much space the writing log file takes up. 0 means no limit. The unit is `MB`. Only take effect when `Rotation` is `size`.
- `Rotation`: represents the type of log rotation rule. Default is `daily`.
  - `daily` rotate the logs by day.
  - `size` rotate the logs by size of logs.

## Logging methods

```go
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
```

- `logger.WithContext(ctx)`: returns a `Logger` that writes the `trace` and `span` ids of `ctx` and the fields `logger.ContextWithFields` stored in it into every entry.
- `Debug`, `Error`, `Info`: write any kind of messages into logs, like `fmt.Sprint(…)`; `LogField` arguments become fields.
- `Debugf`, `Errorf`, `Infof`: write messages with given format into logs.
- `Debugw`, `Errorw`, `Infow`, `Sloww`: write the string message with given `key:value` fields.
- `WithCallerSkip`: skip more stack frames when reporting the caller, for logging helpers.
- `WithDuration`: write elapsed duration into the log messages, with key `duration`.

## Write the logs to specific stores

`logger` defines two functions to let you write logs into any store.

- `logger.NewWriter(w io.Writer)`
- `logger.SetWriter(writer logger.Writer)`

## Filtering sensitive fields

Entries are redacted before they are written. A field whose key names a credential or a personal datum (for
example `token`, `password`, `email`, `phone`) is written as `[REDACTED]`, and JWTs, bot tokens, email addresses,
bearer tokens and secret query parameters are masked inside messages and field values. `logger.RiskField` is the only
way to log the client IP and the user agent, which the risk-control audit needs.
