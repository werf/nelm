package log

import (
	"context"

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

func (l *LogboekLogger) Debug(format string, a ...interface{}) {
	l.logger.Debug().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Info(format string, a ...interface{}) {
	l.logger.Info().LogF(format+"\n", a...)
}

func (l *LogboekLogger) Warn(format string, a ...interface{}) {
	l.logger.Warn().LogF(format+"\n", a...)
}
