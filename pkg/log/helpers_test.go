package log

import "log/slog"

type selfLoggingValue struct {
	logger *slog.Logger
}

func (v selfLoggingValue) LogValue() slog.Value {
	v.logger.Info("nested message")

	return slog.StringValue("value")
}
