package log

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/werf/kubedog/pkg/dyntracker/util"
)

var _ slog.Handler = (*SlogHandler)(nil)

// SlogHandler writes log/slog records through Default, so they respect its level and output streams.
// slog levels map to Debug, Info, Warn and Error of Default.
type SlogHandler struct {
	buf  *util.Concurrent[*bytes.Buffer]
	ctx  context.Context
	text slog.Handler
}

func NewSlogHandler(ctx context.Context) *SlogHandler {
	buf := &bytes.Buffer{}

	return &SlogHandler{
		buf: util.NewConcurrent(buf),
		ctx: ctx,
		text: slog.NewTextHandler(buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
				if len(groups) == 0 && (attr.Key == slog.TimeKey || attr.Key == slog.LevelKey || attr.Key == slog.MessageKey) {
					return slog.Attr{}
				}

				return attr
			},
		}),
	}
}

func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return Default.AcceptLevel(h.ctx, levelFromSlogLevel(level))
}

func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	var attrs string
	if err := h.buf.RWTransactionErr(func(buf *bytes.Buffer) error {
		buf.Reset()

		if err := h.text.Handle(ctx, record); err != nil {
			return fmt.Errorf("format slog record attributes: %w", err)
		}

		attrs = strings.TrimSpace(buf.String())

		return nil
	}); err != nil {
		return err
	}

	msg := record.Message
	if attrs != "" {
		msg += " " + attrs
	}

	switch levelFromSlogLevel(record.Level) {
	case ErrorLevel:
		Default.Error(h.ctx, "%s", msg)
	case WarningLevel:
		Default.Warn(h.ctx, "%s", msg)
	case InfoLevel:
		Default.Info(h.ctx, "%s", msg)
	default:
		Default.Debug(h.ctx, "%s", msg)
	}

	return nil
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{
		buf:  h.buf,
		ctx:  h.ctx,
		text: h.text.WithAttrs(attrs),
	}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{
		buf:  h.buf,
		ctx:  h.ctx,
		text: h.text.WithGroup(name),
	}
}

func levelFromSlogLevel(level slog.Level) Level {
	switch {
	case level >= slog.LevelError:
		return ErrorLevel
	case level >= slog.LevelWarn:
		return WarningLevel
	case level >= slog.LevelInfo:
		return InfoLevel
	default:
		return DebugLevel
	}
}
