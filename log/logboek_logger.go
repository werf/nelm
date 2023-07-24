package log

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/types"
)

func NewLogboekLogger(ctx context.Context) *LogboekLogger {
	return &LogboekLogger{
		logger: logboek.Context(ctx),
	}
}

type LogboekLogger struct {
	logger types.LoggerInterface
}

func (l *LogboekLogger) Trace(format string, a ...interface{}) {
	l.logger.Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) TraceStruct(obj interface{}, format string, a ...interface{}) {
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		l.Warn("error marshaling object to json while tracing struct for %q: %w", fmt.Sprintf(format, a), err)
	}

	l.logger.Debug().LogF(fmt.Sprintf(format+"\n", a...) + string(out) + "\n")
}

func (l *LogboekLogger) Debug(format string, a ...interface{}) {
	l.logger.Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Info(format string, a ...interface{}) {
	l.logger.Info().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Warn(format string, a ...interface{}) {
	l.logger.Warn().LogF(format+"\n", a...)
}

func (l *LogboekLogger) LogBlock(task func() error, format string, a ...interface{}) error {
	return l.logger.LogProcess(format, a...).DoError(task)
}
