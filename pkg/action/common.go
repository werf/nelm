package action

import (
	"context"
	"fmt"
	"io"
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

	"github.com/werf/nelm/pkg/log"
)

func init() {
	style := lo.Must(chroma.NewXMLStyle(strings.NewReader(syntaxHighlightTheme)))
	styles.Register(style)
}

const (
	ReleaseStorageDriverDefault    = ""
	ReleaseStorageDriverSecrets    = "secrets"
	ReleaseStorageDriverSecret     = "secret"
	ReleaseStorageDriverConfigMaps = "configmaps"
	ReleaseStorageDriverConfigMap  = "configmap"
	ReleaseStorageDriverMemory     = "memory"
	ReleaseStorageDriverSQL        = "sql"
)

const (
	YamlOutputFormat  = "yaml"
	JsonOutputFormat  = "json"
	TableOutputFormat = "table"
)

const (
	SilentLogLevel  = string(log.SilentLevel)
	ErrorLogLevel   = string(log.ErrorLevel)
	WarningLogLevel = string(log.WarningLevel)
	InfoLogLevel    = string(log.InfoLevel)
	DebugLogLevel   = string(log.DebugLevel)
	TraceLogLevel   = string(log.TraceLevel)
)

var LogLevels []string = lo.Map(log.Levels, func(lvl log.Level, _ int) string {
	return string(lvl)
})

const (
	DefaultQPSLimit              = 30
	DefaultBurstLimit            = 100
	DefaultNetworkParallelism    = 30
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultReleaseHistoryLimit   = 10
	DefaultLogColorMode          = log.LogColorModeAuto

	StubReleaseName      = "stub-release"
	StubReleaseNamespace = "stub-namespace"
)

var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)

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

func writeWithSyntaxHighlight(outStream io.Writer, text, lang string, colorLevel terminfo.ColorLevel) error {
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

func printNotes(ctx context.Context, notes string) {
	if notes == "" {
		return
	}

	log.Default.InfoBlock(ctx, log.BlockOptions{
		BlockTitle: color.Style{color.Bold, color.Blue}.Render("Release notes"),
	}, func() {
		log.Default.Info(ctx, notes)
	})
}

func saveReport(reportPath string, report *reportV2) error {
	reportByte, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(reportPath, reportByte, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}
