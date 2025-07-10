package action

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"os"

	contdlog "github.com/containerd/log"
	"github.com/davecgh/go-spew/spew"
	"github.com/gookit/color"
	"github.com/sirupsen/logrus"
	"github.com/xo/terminfo"
	"k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/werf/3p-helm/pkg/engine"
	kddebug "github.com/werf/kubedog/pkg/tracker/debug"
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/log"
)

type SetupLoggingOptions struct {
	ColorMode      string
	LogIsParseable bool
}

func SetupLogging(ctx context.Context, logLevel string, opts SetupLoggingOptions) context.Context {
	if val := ctx.Value(log.LogboekLoggerCtxKeyName); val == nil {
		ctx = logboek.NewContext(ctx, logboek.DefaultLogger())
	}

	log.Default.SetLevel(ctx, log.Level(logLevel))

	spew.Config.DisablePointerAddresses = true
	spew.Config.DisableCapacities = true

	switch logLevel {
	case SilentLogLevel, ErrorLogLevel, WarningLogLevel, InfoLogLevel:
		stdlog.SetOutput(io.Discard)

		klog.SetOutputBySeverity("FATAL", ioutil.Discard)
		klog.SetOutputBySeverity("ERROR", ioutil.Discard)
		klog.SetOutputBySeverity("WARNING", ioutil.Discard)
		klog.SetOutputBySeverity("INFO", ioutil.Discard)

		klogv2.SetOutputBySeverity("FATAL", ioutil.Discard)
		klogv2.SetOutputBySeverity("ERROR", ioutil.Discard)
		klogv2.SetOutputBySeverity("WARNING", ioutil.Discard)
		klogv2.SetOutputBySeverity("INFO", ioutil.Discard)

		logrus.SetOutput(ioutil.Discard)

		contdlog.L.Logger.SetOutput(ioutil.Discard)

		engine.Debug = false

		kddebug.SetDebug(false)
	case DebugLogLevel:
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

		contdlog.L.Logger.SetOutput(logboek.Context(ctx).OutStream())
		contdlog.L.Logger.SetLevel(logrus.DebugLevel)

		engine.Debug = true

		kddebug.SetDebug(true)
	case TraceLogLevel:
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

		contdlog.L.Logger.SetOutput(logboek.Context(ctx).OutStream())
		contdlog.L.Logger.SetLevel(logrus.TraceLevel)

		engine.Debug = true

		kddebug.SetDebug(true)
	default:
		panic(fmt.Sprintf("unknown log level %q", logLevel))
	}

	colorLevel := getColorLevel(opts.ColorMode, opts.LogIsParseable)

	color.Enable = colorLevel != terminfo.ColorLevelNone
	color.ForceSetColorLevel(colorLevel)

	return ctx
}
