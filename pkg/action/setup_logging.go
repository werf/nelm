package action

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

	kdlog "github.com/werf/kubedog/pkg/log"
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/helm/pkg/engine"
	"github.com/werf/nelm/pkg/log"
)

type SetupLoggingOptions struct {
	ColorMode      string
	LogIsParseable bool
}

func SetupLogging(ctx context.Context, logLevel log.Level, opts SetupLoggingOptions) context.Context {
	if val := ctx.Value(log.LogboekLoggerCtxKeyName); val == nil {
		ctx = logboek.NewContext(ctx, logboek.DefaultLogger())
	}

	log.Default.SetLevel(ctx, logLevel)

	spew.Config.DisablePointerAddresses = true
	spew.Config.DisableCapacities = true

	switch logLevel {
	case log.SilentLevel, log.ErrorLevel, log.WarningLevel, log.InfoLevel:
		stdlog.SetOutput(io.Discard)

		klog.SetOutput(io.Discard)

		klogFlags := &flag.FlagSet{}
		klog.InitFlags(klogFlags)
		lo.Must0(klogFlags.Set("logtostderr", "false"))
		lo.Must0(klogFlags.Set("alsologtostderr", "false"))
		lo.Must0(klogFlags.Set("stderrthreshold", "4"))

		klogv2.SetOutput(io.Discard)

		klogV2Flags := &flag.FlagSet{}
		klogv2.InitFlags(klogV2Flags)
		lo.Must0(klogV2Flags.Set("logtostderr", "false"))
		lo.Must0(klogV2Flags.Set("alsologtostderr", "false"))
		lo.Must0(klogV2Flags.Set("stderrthreshold", "4"))

		logrus.SetOutput(io.Discard)

		cdlog.L.Logger.SetOutput(io.Discard)

		engine.Debug = false

		kdlog.SetDebug(false)
	case log.DebugLevel:
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

		kdlog.SetDebug(true)
	case log.TraceLevel:
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

		kdlog.SetDebug(true)
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
	case log.LogColorModeOff:
		return terminfo.ColorLevelNone
	case log.LogColorModeOn:
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
