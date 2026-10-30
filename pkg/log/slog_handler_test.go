package log

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/werf/logboek"
)

type selfLoggingValue struct {
	logger *slog.Logger
}

func (v selfLoggingValue) LogValue() slog.Value {
	v.logger.Info("nested message")

	return slog.StringValue("value")
}

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

func TestSlogHandlerAllowsLoggingFromLogValuer(t *testing.T) {
	prevDefault := Default
	t.Cleanup(func() { Default = prevDefault })

	Default = NewLogboekLogger()

	var outBuf, errBuf bytes.Buffer

	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&outBuf, &errBuf))
	Default.SetLevel(ctx, InfoLevel)

	logger := slog.New(NewSlogHandler(ctx)).With("chart", "parent")

	done := make(chan struct{})
	go func() {
		logger.Info("outer message", "key", selfLoggingValue{logger: logger})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("logging from a LogValuer deadlocked")
	}

	assert.Equal(t, "nested message chart=parent\nouter message chart=parent key=value\n", outBuf.String())
}
