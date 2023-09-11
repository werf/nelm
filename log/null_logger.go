package log

import "context"

var _ Logger = (*NullLogger)(nil)

func NewNullLogger() *NullLogger {
	return &NullLogger{}
}

type NullLogger struct{}

func (l *NullLogger) Trace(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{}) {
}

func (l *NullLogger) Debug(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) Info(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) Warn(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) Error(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) LogBlock(ctx context.Context, task func() error, format string, a ...interface{}) error {
	return nil
}
