package logger

// A LogConf is a logging config. The defaults below are the ones the
// configuration loader fills in for a missing key (the default tags); a
// LogConf built in code without them keeps the zero-value behaviour noted
// where it differs.
type LogConf struct {
	// ServiceName represents the service name, default is `PPanel`. In
	// `volume` mode it names the log directory.
	ServiceName string `yaml:"ServiceName" default:"PPanel"`
	// Mode represents the logging mode, default is `file`; an empty mode
	// logs to the console.
	// console: log to console.
	// file: log to file.
	// volume: used in k8s, prepend the hostname to the log file name.
	Mode string `yaml:"Mode" default:"file"`
	// Encoding represents the encoding type, default is `json`.
	// json: json encoding.
	// plain: plain text encoding, typically used in development.
	Encoding string `yaml:"Encoding" default:"json"`
	// TimeFormat represents the time format of log entries, default is
	// `2006-01-02 15:04:05.000`; an empty format keeps
	// `2006-01-02T15:04:05.000Z07:00`.
	TimeFormat string `yaml:"TimeFormat" default:"2006-01-02 15:04:05.000"`
	// Path represents the log file path, default is `logs`.
	Path string `yaml:"Path" default:"logs"`
	// Level represents the log level, default is `info`; an empty level
	// logs everything, debug included.
	Level string `yaml:"Level" default:"info"`
	// MaxContentLength represents the max bytes per textual log value,
	// default is 16384; zero does not truncate.
	MaxContentLength uint32 `yaml:"MaxContentLength" default:"16384"`
	// Compress represents whether to compress the log file, default is `false`.
	Compress bool `yaml:"Compress" default:"false"`
	// Stat is kept so existing configuration files still load; the server
	// writes no statistics logs, so it has no effect.
	Stat bool `yaml:"Stat" default:"true"`
	// KeepDays represents how many days the log files will be kept. Defaults to 30 days.
	// Only take effect when Mode is `file` or `volume`, both work when Rotation is `daily` or `size`.
	KeepDays int `yaml:"KeepDays" default:"30"`
	// StackCooldownMillis represents the cooldown time for stack logging, default is 100ms.
	StackCooldownMillis int `yaml:"StackCooldownMillis" default:"100"`
	// MaxBackups represents how many backup log files will be kept. Defaults to 30.
	// Only take effect when RotationRuleType is `size`.
	// Even though `MaxBackups` sets 0, log files will still be removed
	// if the `KeepDays` limitation is reached.
	MaxBackups int `yaml:"MaxBackups" default:"30"`
	// MaxSize represents how much space the writing log file takes up. Defaults to 100 MB.
	// Only take effect when RotationRuleType is `size`
	MaxSize int `yaml:"MaxSize" default:"100"`
	// Rotation represents the type of log rotation rule. Default is `daily`.
	// daily: daily rotation.
	// size: size limited rotation.
	Rotation string `yaml:"Rotation" default:"daily"`
	// FileTimeFormat represents the time format for file name, default is `2006-01-02T15:04:05.000Z07:00`.
	FileTimeFormat string `yaml:"FileTimeFormat" default:"2006-01-02T15:04:05.000Z07:00"`
}
