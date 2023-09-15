package log

import (
	"context"

	"github.com/werf/logboek/pkg/types"
)

type Logger interface {
	Trace(ctx context.Context, format string, a ...interface{})
	TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{})
	Debug(ctx context.Context, format string, a ...interface{})
	Info(ctx context.Context, format string, a ...interface{})
	Warn(ctx context.Context, format string, a ...interface{})
	Error(ctx context.Context, format string, a ...interface{})
	InfoBlock(ctx context.Context, format string, a ...interface{}) types.LogBlockInterface
	InfoProcess(ctx context.Context, format string, a ...interface{}) types.LogProcessInterface
}
