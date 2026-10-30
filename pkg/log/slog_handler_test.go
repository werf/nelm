package log

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/logboek"
)

func TestSlogHandler(t *testing.T) {
	type logOutput struct {
		err string
		out string
	}

	logAll := func(logger *slog.Logger) {
		logger.Debug("debug message", "key", "value")
		logger.Info("info message")
		logger.Warn("warn message", "error", "has spaces")
		logger.Error("error message")
	}

	tests := []struct {
		name   string
		level  Level
		log    func(logger *slog.Logger)
		expect logOutput
	}{
		{
			name:  "debug level shows all messages, warnings and errors in the error stream",
			level: DebugLevel,
			log:   logAll,
			expect: logOutput{
				err: "warn message error=\"has spaces\"\nerror message\n",
				out: "debug message key=value\ninfo message\n",
			},
		},
		{
			name:  "info level hides debug messages",
			level: InfoLevel,
			log:   logAll,
			expect: logOutput{
				err: "warn message error=\"has spaces\"\nerror message\n",
				out: "info message\n",
			},
		},
		{
			name:  "error level shows only errors",
			level: ErrorLevel,
			log:   logAll,
			expect: logOutput{
				err: "error message\n",
			},
		},
		{
			name:   "silent level shows nothing",
			level:  SilentLevel,
			log:    logAll,
			expect: logOutput{},
		},
		{
			name:  "attributes and groups of derived loggers are kept",
			level: InfoLevel,
			log: func(logger *slog.Logger) {
				logger.With("chart", "parent").WithGroup("dep").Warn("invalid definition", "name", "sub")
			},
			expect: logOutput{
				err: "invalid definition chart=parent dep.name=sub\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevDefault := Default
			t.Cleanup(func() { Default = prevDefault })

			Default = NewLogboekLogger()

			var outBuf, errBuf bytes.Buffer

			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&outBuf, &errBuf))
			Default.SetLevel(ctx, tt.level)

			tt.log(slog.New(NewSlogHandler(ctx)))

			got := logOutput{err: errBuf.String(), out: outBuf.String()}
			assert.Equal(t, tt.expect, got)
		})
	}
}
