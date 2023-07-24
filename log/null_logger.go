package log

func NewNullLogger() *NullLogger {
	return &NullLogger{}
}

type NullLogger struct{}

func (l *NullLogger) Trace(format string, a ...interface{}) {}

func (l *NullLogger) TraceStruct(obj interface{}, format string, a ...interface{}) {}

func (l *NullLogger) Debug(format string, a ...interface{}) {}

func (l *NullLogger) Info(format string, a ...interface{}) {}

func (l *NullLogger) Warn(format string, a ...interface{}) {}

func (l *NullLogger) LogBlock(task func() error, format string, a ...interface{}) error {
	return nil
}
