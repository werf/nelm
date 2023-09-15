package log

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/types"
)

var _ Logger = (*LogboekLogger)(nil)

func NewLogboekLogger() *LogboekLogger {
	return &LogboekLogger{}
}

type LogboekLogger struct{}

func (l *LogboekLogger) Trace(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{}) {
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		l.Warn(ctx, "error marshaling object to json while tracing struct for %q: %w", fmt.Sprintf(format, a), err)
	}

	logboek.Context(ctx).Debug().LogF(fmt.Sprintf(format+"\n", a...) + string(out) + "\n")
}

func (l *LogboekLogger) Debug(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Info(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Default().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Warn(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Warn().LogFHighlight(format+"\n", a...)
}

func (l *LogboekLogger) Error(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Error().LogFHighlight(format+"\n", a...)
}

func (l *LogboekLogger) InfoBlock(ctx context.Context, format string, a ...interface{}) types.LogBlockInterface {
	return logboek.Context(ctx).Default().LogBlock(format, a...)
}

func (l *LogboekLogger) InfoProcess(ctx context.Context, format string, a ...interface{}) types.LogProcessInterface {
	return logboek.Context(ctx).Default().LogProcess(format, a...)
}
