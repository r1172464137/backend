## logger 配置

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

- `ServiceName`：设置服务名称，可选。在 `volume` 模式下，该名称用于生成日志文件。
- `Mode`：输出日志的模式，默认是 `console`
  - `console` 模式将日志写到 `stdout/stderr`
  - `file` 模式将日志写到 `Path` 指定目录的文件中
  - `volume` 模式在 docker 中使用，将日志写入挂载的卷中
- `Encoding`: 指示如何对日志进行编码，默认是 `json`
  - `json`模式以 json 格式写日志
  - `plain`模式用纯文本写日志，并带有终端颜色显示
- `TimeFormat`：自定义时间格式，可选。默认是 `2006-01-02T15:04:05.000Z07:00`
- `Path`：设置日志路径，默认为 `logs`
- `Level`: 用于过滤日志的日志级别。默认为 `info`
  - `info`，所有日志都被写入
  - `error`, `info` 的日志被丢弃
  - `severe`, `info` 和 `error` 日志被丢弃，只有 `severe` 日志被写入
- `Compress`: 是否压缩日志文件，只在 `file` 模式下工作
- `KeepDays`：日志文件被保留多少天，在给定的天数之后，过期的文件将被自动删除。对 `console` 模式没有影响
- `StackCooldownMillis`：多少毫秒后再次写入堆栈跟踪。用来避免堆栈跟踪日志过多
- `MaxBackups`: 多少个日志文件备份将被保存。0代表所有备份都被保存。当`Rotation`被设置为`size`时才会起作用。注意：`KeepDays`选项的优先级会比`MaxBackups`高，即使`MaxBackups`被设置为0，当达到`KeepDays`上限时备份文件同样会被删除。
- `MaxSize`: 当前被写入的日志文件最大可占用多少空间。0代表没有上限。单位为`MB`。当`Rotation`被设置为`size`时才会起作用。
- `Rotation`: 日志轮转策略类型。默认为`daily`（按天轮转）。
  - `daily` 按天轮转。
  - `size` 按日志大小轮转。


## 打印日志方法

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

- `logger.WithContext(ctx)`：返回的 `Logger` 会把 `ctx` 的 `trace`、`span` id 以及 `logger.ContextWithFields` 存入的字段写入每条日志
- `Debug`, `Error`, `Info`: 将任何类型的信息写进日志，使用 `fmt.Sprint(...)` 来转换为 `string`；`LogField` 参数作为字段写入
- `Debugf`, `Errorf`, `Infof`: 将指定格式的信息写入日志
- `Debugw`, `Errorw`, `Infow`, `Sloww`: 写日志，并带上给定的 `key:value` 字段
- `WithCallerSkip`：报告调用位置时多跳过若干栈帧，用于封装日志的辅助函数
- `WithDuration`: 将指定的时间写入日志信息中，字段名为 `duration`

## 将日志写到指定的存储

`logger` 提供了两个函数，方便将日志写入任何存储。

- `logger.NewWriter(w io.Writer)`
- `logger.SetWriter(writer logger.Writer)`

## 过滤敏感字段

日志在写出之前会先脱敏：键名表示凭据或个人信息（例如 `token`、`password`、`email`、`phone`）的字段写为
`[REDACTED]`；消息和字段值中的 JWT、机器人令牌、邮箱地址、Bearer 令牌和敏感查询参数会被遮盖。
`logger.RiskField` 是记录客户端 IP 和 User-Agent 的唯一方式，供风控审计使用。
