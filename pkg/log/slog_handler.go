package log

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
)

var _ slog.Handler = (*SlogHandler)(nil)

// SlogHandler writes log/slog records through Default, so they respect its level and output streams.
// slog levels map to Debug, Info, Warn and Error of Default.
type SlogHandler struct {
	ctx context.Context
	// Attributes and groups are replayed on a fresh text handler for every record, so that no state
	// is shared between records and a LogValuer may log itself.
	wrappers []func(slog.Handler) slog.Handler
}

func NewSlogHandler(ctx context.Context) *SlogHandler {
	return &SlogHandler{ctx: ctx}
}

func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return Default.AcceptLevel(h.ctx, levelFromSlogLevel(level))
}

func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	var buf bytes.Buffer

	var text slog.Handler = slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && (attr.Key == slog.TimeKey || attr.Key == slog.LevelKey || attr.Key == slog.MessageKey) {
				return slog.Attr{}
			}

			return attr
		},
	})
	for _, wrap := range h.wrappers {
		text = wrap(text)
	}

	if err := text.Handle(ctx, record); err != nil {
		return fmt.Errorf("format slog record attributes: %w", err)
	}

	msg := record.Message
	if attrs := strings.TrimSpace(buf.String()); attrs != "" {
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
	return h.withWrapper(func(handler slog.Handler) slog.Handler {
		return handler.WithAttrs(attrs)
	})
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return h.withWrapper(func(handler slog.Handler) slog.Handler {
		return handler.WithGroup(name)
	})
}

func (h *SlogHandler) withWrapper(wrap func(slog.Handler) slog.Handler) *SlogHandler {
	return &SlogHandler{
		ctx:      h.ctx,
		wrappers: append(append([]func(slog.Handler) slog.Handler{}, h.wrappers...), wrap),
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
