package log

type Logger interface {
	Trace(format string, a ...interface{})
	TraceStruct(obj interface{}, format string, a ...interface{})
	Debug(format string, a ...interface{})
	Info(format string, a ...interface{})
	Warn(format string, a ...interface{})
	LogBlock(task func() error, format string, a ...interface{}) error
}
