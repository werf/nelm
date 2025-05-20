package log

import (
	"context"
)

var _ Logger = (*NullLogger)(nil)

func NewNullLogger() *NullLogger {
	return &NullLogger{}
}

type NullLogger struct{}

func (l *NullLogger) Trace(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{}) {
}

func (l *NullLogger) TracePush(ctx context.Context, group, format string, a ...interface{}) {}

func (l *NullLogger) TracePop(ctx context.Context, group string) {}

func (l *NullLogger) Debug(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) DebugPush(ctx context.Context, group, format string, a ...interface{}) {}

func (l *NullLogger) DebugPop(ctx context.Context, group string) {}

func (l *NullLogger) Info(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) InfoPush(ctx context.Context, group, format string, a ...interface{}) {}

func (l *NullLogger) InfoPop(ctx context.Context, group string) {}

func (l *NullLogger) Warn(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) WarnPush(ctx context.Context, group, format string, a ...interface{}) {}

func (l *NullLogger) WarnPop(ctx context.Context, group string) {}

func (l *NullLogger) Error(ctx context.Context, format string, a ...interface{}) {}

func (l *NullLogger) ErrorPush(ctx context.Context, group, format string, a ...interface{}) {}

func (l *NullLogger) ErrorPop(ctx context.Context, group string) {}

func (l *NullLogger) InfoBlock(ctx context.Context, opts BlockOptions, fn func() error) error {
	return nil
}

func (l *NullLogger) SetLevel(ctx context.Context, lvl Level) {}

func (l *NullLogger) Level(context.Context) Level {
	return InfoLevel
}

func (l *NullLogger) AcceptLevel(ctx context.Context, lvl Level) bool {
	return false
}
