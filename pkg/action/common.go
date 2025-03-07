package action

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/pkg/homedir"
	"github.com/gookit/color"
	"github.com/samber/lo"
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

	if err := quick.Highlight(outStream, text, lang, formatterName, syntaxHighlightThemeName); err != nil {
		return fmt.Errorf("highlight and write to output: %w", err)
	}

	return nil
}

func init() {
	style := lo.Must(chroma.NewXMLStyle(strings.NewReader(syntaxHighlightTheme)))
	styles.Register(style)
}

const syntaxHighlightThemeName = "solarized-dark-customized"

var syntaxHighlightTheme = fmt.Sprintf(`
<style name=%q>
  <entry type="Other" style="#9c4c2a"/>
  <entry type="Keyword" style="#719e07"/>
  <entry type="KeywordConstant" style="#9c4c2a"/>
  <entry type="KeywordDeclaration" style="#3f7541"/>
  <entry type="KeywordReserved" style="#3f7541"/>
  <entry type="KeywordType" style="#a14240"/>
  <entry type="NameBuiltin" style="#b58900"/>
  <entry type="NameBuiltinPseudo" style="#018727"/>
  <entry type="NameClass" style="#3f7541"/>
  <entry type="NameConstant" style="#9c4c2a"/>
  <entry type="NameDecorator" style="#3f7541"/>
  <entry type="NameEntity" style="#9c4c2a"/>
  <entry type="NameException" style="#9c4c2a"/>
  <entry type="NameFunction" style="#3f7541"/>
  <entry type="NameTag" style="#3f7541"/>
  <entry type="NameVariable" style="#3f7541"/>
  <entry type="LiteralStringBacktick" style="#586e75"/>
  <entry type="LiteralStringChar" style="#328a82"/>
  <entry type="LiteralStringEscape" style="#9c4c2a"/>
  <entry type="LiteralStringRegex" style="#a14240"/>
  <entry type="LiteralNumber" style="#328a82"/>
  <entry type="Operator" style="#719e07"/>
  <entry type="Comment" style="#586e75"/>
  <entry type="CommentSpecial" style="#719e07"/>
  <entry type="CommentPreproc" style="#719e07"/>
  <entry type="GenericDeleted" style="#a14240"/>
  <entry type="GenericEmph" style="italic"/>
  <entry type="GenericError" style="bold #a14240"/>
  <entry type="GenericHeading" style="#9c4c2a"/>
  <entry type="GenericInserted" style="#719e07"/>
  <entry type="GenericStrong" style="bold"/>
  <entry type="GenericSubheading" style="#3f7541"/>
</style>
`, syntaxHighlightThemeName)
