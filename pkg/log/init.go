package log

import (
	"context"
	"flag"
	"fmt"
	"io"
	stdlog "log"
	"os"

	cdlog "github.com/containerd/log"
	"github.com/davecgh/go-spew/spew"
	"github.com/gookit/color"
	"github.com/hofstadter-io/cinful"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/xo/terminfo"
	"k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/werf/3p-helm/pkg/engine"
	"github.com/werf/kubedog/pkg/tracker/debug"
	"github.com/werf/logboek"
)

var Default Logger = NewLogboekLogger()

type SetupLoggingOptions struct {
	ColorMode      string
	LogIsParseable bool
}

// Sets up logging levels, colors, output formats, etc.
func SetupLogging(ctx context.Context, logLevel Level, opts SetupLoggingOptions) context.Context {
	if val := ctx.Value(LogboekLoggerCtxKeyName); val == nil {
		ctx = logboek.NewContext(ctx, logboek.DefaultLogger())
	}

	Default.SetLevel(ctx, logLevel)

	spew.Config.DisablePointerAddresses = true
	spew.Config.DisableCapacities = true

	switch logLevel {
	case SilentLevel, ErrorLevel, WarningLevel, InfoLevel:
		stdlog.SetOutput(io.Discard)

		klog.SetOutput(io.Discard)
		// From: https://github.com/kubernetes/klog/issues/87#issuecomment-1671820147
		klogFlags := &flag.FlagSet{}
		klog.InitFlags(klogFlags)
		lo.Must0(klogFlags.Set("logtostderr", "false"))
		lo.Must0(klogFlags.Set("alsologtostderr", "false"))
		lo.Must0(klogFlags.Set("stderrthreshold", "4"))

		klogv2.SetOutput(io.Discard)
		// From: https://github.com/kubernetes/klog/issues/87#issuecomment-1671820147
		klogV2Flags := &flag.FlagSet{}
		klogv2.InitFlags(klogV2Flags)
		lo.Must0(klogV2Flags.Set("logtostderr", "false"))
		lo.Must0(klogV2Flags.Set("alsologtostderr", "false"))
		lo.Must0(klogV2Flags.Set("stderrthreshold", "4"))

		logrus.SetOutput(io.Discard)

		cdlog.L.Logger.SetOutput(io.Discard)

		engine.Debug = false

		debug.SetDebug(false)
	case DebugLevel:
		stdlog.SetOutput(os.Stdout)

		klog.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("ERROR", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("WARNING", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("INFO", logboek.Context(ctx).OutStream())

		klogv2.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("ERROR", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("WARNING", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("INFO", logboek.Context(ctx).OutStream())

		logrus.SetOutput(logboek.Context(ctx).OutStream())
		logrus.SetLevel(logrus.DebugLevel)

		cdlog.L.Logger.SetOutput(logboek.Context(ctx).OutStream())
		cdlog.L.Logger.SetLevel(logrus.DebugLevel)

		engine.Debug = true

		debug.SetDebug(true)
	case TraceLevel:
		stdlog.SetOutput(os.Stdout)

		klog.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("ERROR", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("WARNING", logboek.Context(ctx).ErrStream())
		klog.SetOutputBySeverity("INFO", logboek.Context(ctx).OutStream())

		klogv2.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("ERROR", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("WARNING", logboek.Context(ctx).ErrStream())
		klogv2.SetOutputBySeverity("INFO", logboek.Context(ctx).OutStream())

		logrus.SetOutput(logboek.Context(ctx).OutStream())
		logrus.SetLevel(logrus.TraceLevel)

		cdlog.L.Logger.SetOutput(logboek.Context(ctx).OutStream())
		cdlog.L.Logger.SetLevel(logrus.TraceLevel)

		engine.Debug = true

		debug.SetDebug(true)
	default:
		panic(fmt.Sprintf("unknown log level %q", logLevel))
	}

	colorLevel := getColorLevel(opts.ColorMode, opts.LogIsParseable)

	color.Enable = colorLevel != terminfo.ColorLevelNone
	color.ForceSetColorLevel(colorLevel)

	return ctx
}

func getColorLevel(mode string, logIsParseable bool) terminfo.ColorLevel {
	switch mode {
	case LogColorModeOff:
		return terminfo.ColorLevelNone
	case LogColorModeOn:
		if colorLevel := color.DetectColorLevel(); colorLevel == terminfo.ColorLevelNone {
			return terminfo.ColorLevelHundreds
		} else {
			return colorLevel
		}
	}

	if ciInfo := cinful.Info(); ciInfo != nil {
		switch ciInfo.Constant {
		case "GITLAB", "GITHUB_ACTIONS":
			if logIsParseable {
				return terminfo.ColorLevelNone
			} else {
				return terminfo.ColorLevelHundreds
			}
		case "JENKINS":
			if logIsParseable {
				return terminfo.ColorLevelNone
			} else {
				switch os.Getenv("TERM") {
				// From https://github.com/jenkinsci/ansicolor-plugin/tree/e2a42bf6c6acadc46468a6bf75dbd958a4747d0b?tab=readme-ov-file#colormaps
				case "xterm", "vga", "gnome-terminal", "css":
					return terminfo.ColorLevelHundreds
				}
			}
		default:
			if logIsParseable {
				return terminfo.ColorLevelNone
			} else {
				return color.DetectColorLevel()
			}
		}
	}

	if piped, err := stdoutPiped(); err != nil || piped {
		return terminfo.ColorLevelNone
	}

	return color.DetectColorLevel()
}

func stdoutPiped() (bool, error) {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false, fmt.Errorf("get stdout fileinfo: %w", err)
	}

	piped := (fileInfo.Mode() & os.ModeCharDevice) == 0

	return piped, nil
}
