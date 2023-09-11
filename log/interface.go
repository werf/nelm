package log

import "context"

type Logger interface {
	Trace(ctx context.Context, format string, a ...interface{})
	TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{})
	Debug(ctx context.Context, format string, a ...interface{})
	Info(ctx context.Context, format string, a ...interface{})
	Warn(ctx context.Context, format string, a ...interface{})
	Error(ctx context.Context, format string, a ...interface{})
	LogBlock(ctx context.Context, task func() error, format string, a ...interface{}) error
}
