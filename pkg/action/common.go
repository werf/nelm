package action

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/pkg/homedir"
	"github.com/gookit/color"
	"github.com/xo/terminfo"
	"k8s.io/klog"
	klog_v2 "k8s.io/klog/v2"

	"github.com/werf/kubedog/pkg/display"
	"github.com/werf/logboek"
)

type LogColorMode string

const (
	LogColorModeUnspecified LogColorMode = ""
	LogColorModeAuto        LogColorMode = "auto"
	LogColorModeOff         LogColorMode = "off"
	LogColorModeOn          LogColorMode = "on"
)

const (
	SyntaxHighlightTheme = "solarized-light"
)

type ReleaseStorageDriver string

const (
	ReleaseStorageDriverDefault    ReleaseStorageDriver = ""
	ReleaseStorageDriverSecrets    ReleaseStorageDriver = "secrets"
	ReleaseStorageDriverSecret     ReleaseStorageDriver = "secret"
	ReleaseStorageDriverConfigMaps ReleaseStorageDriver = "configmaps"
	ReleaseStorageDriverConfigMap  ReleaseStorageDriver = "configmap"
	ReleaseStorageDriverMemory     ReleaseStorageDriver = "memory"
	ReleaseStorageDriverSQL        ReleaseStorageDriver = "sql"
)

const (
	DefaultQPSLimit              = 30
	DefaultBurstLimit            = 100
	DefaultNetworkParallelism    = 30
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultReleaseHistoryLimit   = 10
	DefaultLogColorMode          = LogColorModeAuto
)

var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)

func initKubedog(ctx context.Context) error {
	flag.CommandLine.Parse([]string{})

	display.SetOut(logboek.Context(ctx).OutStream())
	display.SetErr(logboek.Context(ctx).ErrStream())

	if err := silenceKlog(ctx); err != nil {
		return fmt.Errorf("silence klog: %w", err)
	}

	if err := silenceKlogV2(ctx); err != nil {
		return fmt.Errorf("silence klog v2: %w", err)
	}

	return nil
}

func silenceKlog(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klog.SetOutputBySeverity("INFO", ioutil.Discard)
	klog.SetOutputBySeverity("WARNING", ioutil.Discard)
	klog.SetOutputBySeverity("ERROR", ioutil.Discard)
	klog.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}

func silenceKlogV2(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klog_v2.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klog_v2.SetOutputBySeverity("INFO", ioutil.Discard)
	klog_v2.SetOutputBySeverity("WARNING", ioutil.Discard)
	klog_v2.SetOutputBySeverity("ERROR", ioutil.Discard)
	klog_v2.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}

func stdoutPiped() (bool, error) {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false, fmt.Errorf("get stdout fileinfo: %w", err)
	}

	piped := (fileInfo.Mode() & os.ModeCharDevice) == 0

	return piped, nil
}

func applyLogColorModeDefault(logColorMode LogColorMode, outputToFile bool) LogColorMode {
	if logColorMode == LogColorModeUnspecified || logColorMode == LogColorModeAuto {
		piped, err := stdoutPiped()
		if err != nil {
			return LogColorModeOff
		}

		uncoloredTerminal := color.DetectColorLevel() == terminfo.ColorLevelNone

		if outputToFile || piped || uncoloredTerminal {
			logColorMode = LogColorModeOff
		} else {
			logColorMode = LogColorModeOn
		}
	}

	return logColorMode
}

func writeWithSyntaxHighlight(outStream io.Writer, text string, lang string, colorLevel terminfo.ColorLevel) error {
	if colorLevel == color.LevelNo {
		if _, err := outStream.Write([]byte(text)); err != nil {
			return fmt.Errorf("write to output: %w", err)
		}

		return nil
	}

	var formatterName string
	switch colorLevel {
	case color.Level16:
		formatterName = "terminal16"
	case color.Level256:
		formatterName = "terminal256"
	case color.LevelRgb:
		formatterName = "terminal16m"
	default:
		panic(fmt.Sprintf("unexpected color level %d", colorLevel))
	}

	if err := quick.Highlight(outStream, text, lang, formatterName, SyntaxHighlightTheme); err != nil {
		return fmt.Errorf("highlight and write to output: %w", err)
	}

	return nil
}
