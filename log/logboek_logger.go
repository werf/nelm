package log

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/werf/logboek"
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
	logboek.Context(ctx).Warn().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Error(ctx context.Context, format string, a ...interface{}) {
	logboek.Context(ctx).Error().LogF(format+"\n", a...)
}

func (l *LogboekLogger) LogBlock(ctx context.Context, task func() error, format string, a ...interface{}) error {
	return logboek.Context(ctx).LogProcess(format, a...).DoError(task)
}
