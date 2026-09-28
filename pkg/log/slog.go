package log

import (
	"context"
	"log/slog"
	"strings"
)

var _ slog.Handler = (*SlogHandler)(nil)

// SlogHandler routes slog records from the vendored Helm packages into Default, so their
// debug output is governed by the same level as the rest of nelm instead of being dropped
// by slog's stdlib default handler.
type SlogHandler struct {
	attrs   []slog.Attr
	context context.Context
}

func NewSlogHandler(ctx context.Context) *SlogHandler {
	return &SlogHandler{context: ctx}
}

func (h *SlogHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return Default.AcceptLevel(h.context, slogLevelToLevel(lvl))
}

func (h *SlogHandler) Handle(_ context.Context, record slog.Record) error {
	var b strings.Builder
	b.WriteString(record.Message)

	appendAttr := func(attr slog.Attr) {
		b.WriteString(" ")
		b.WriteString(attr.Key)
		b.WriteString("=")
		b.WriteString(attr.Value.String())
	}

	for _, attr := range h.attrs {
		appendAttr(attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		appendAttr(attr)

		return true
	})

	message := strings.ReplaceAll(b.String(), "%", "%%")

	switch slogLevelToLevel(record.Level) {
	case ErrorLevel:
		Default.Error(h.context, message)
	case WarningLevel:
		Default.Warn(h.context, message)
	case InfoLevel:
		Default.Info(h.context, message)
	default:
		Default.Debug(h.context, message)
	}

	return nil
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{attrs: append(append([]slog.Attr{}, h.attrs...), attrs...), context: h.context}
}

func (h *SlogHandler) WithGroup(string) slog.Handler {
	return h
}

func slogLevelToLevel(lvl slog.Level) Level {
	switch {
	case lvl >= slog.LevelError:
		return ErrorLevel
	case lvl >= slog.LevelWarn:
		return WarningLevel
	case lvl >= slog.LevelInfo:
		return InfoLevel
	default:
		return DebugLevel
	}
}
