package log

import (
	"context"

	"github.com/werf/logboek/pkg/types"
)

type Logger interface {
	Trace(ctx context.Context, format string, a ...interface{})
	TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{})
	TracePush(ctx context.Context, group, format string, a ...interface{})
	TracePop(ctx context.Context, group string)
	Debug(ctx context.Context, format string, a ...interface{})
	DebugPush(ctx context.Context, group, format string, a ...interface{})
	DebugPop(ctx context.Context, group string)
	Info(ctx context.Context, format string, a ...interface{})
	InfoPush(ctx context.Context, group, format string, a ...interface{})
	InfoPop(ctx context.Context, group string)
	Warn(ctx context.Context, format string, a ...interface{})
	WarnPush(ctx context.Context, group, format string, a ...interface{})
	WarnPop(ctx context.Context, group string)
	Error(ctx context.Context, format string, a ...interface{})
	ErrorPush(ctx context.Context, group, format string, a ...interface{})
	ErrorPop(ctx context.Context, group string)
	InfoBlock(ctx context.Context, format string, a ...interface{}) types.LogBlockInterface
	InfoProcess(ctx context.Context, format string, a ...interface{}) types.LogProcessInterface
	SetLevel(ctx context.Context, lvl Level)
	Level(ctx context.Context) Level
	AcceptLevel(ctx context.Context, lvl Level) bool
}

type Level string

const (
	SilentLevel  Level = "silent"
	ErrorLevel   Level = "error"
	WarningLevel Level = "warning"
	InfoLevel    Level = "info"
	DebugLevel   Level = "debug"
	TraceLevel   Level = "trace"
)

var Levels = []Level{SilentLevel, ErrorLevel, WarningLevel, InfoLevel, DebugLevel, TraceLevel}
