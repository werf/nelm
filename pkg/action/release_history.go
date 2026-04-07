package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	helmreleasestatus "github.com/werf/nelm/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/release"
)

const (
	DefaultReleaseHistoryLogLevel     = log.ErrorLevel
	DefaultReleaseHistoryOutputFormat = common.OutputFormatTable
)

type ReleaseHistoryOptions struct {
	common.KubeConnectionOptions

	// Max limits the number of revisions returned. 0 means no limit.
	Max int
	// OutputFormat specifies the output format for the release history.
	// Valid values: "table" (default), "yaml", "json".
	// Defaults to DefaultReleaseHistoryOutputFormat (table) if not specified.
	OutputFormat string
	// OutputNoPrint, when true, suppresses printing the output and only returns the result data structure.
	// Useful when calling this programmatically.
	OutputNoPrint bool
	// ReleaseStorageDriver specifies how release metadata is stored in Kubernetes.
	// Valid values: "secret" (default), "configmap", "sql".
	// Defaults to "secret" if not specified or set to "default".
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
}

type ReleaseHistoryResultV1 struct {
	APIVersion string                         `json:"apiVersion"`
	Releases   []*ReleaseHistoryResultRelease `json:"releases"`
}

type ReleaseHistoryResultRelease struct {
	Name        string                       `json:"name"`
	Namespace   string                       `json:"namespace"`
	Revision    int                          `json:"revision"`
	Status      helmreleasestatus.Status     `json:"status"`
	Updated     *ReleaseHistoryResultUpdated `json:"updated"`
	Annotations map[string]string            `json:"annotations"`
	Chart       *ReleaseHistoryResultChart   `json:"chart"`
	Description string                       `json:"description"`
}

type ReleaseHistoryResultUpdated struct {
	Human      string `json:"human"`
	HumanTable string `json:"-" yaml:"-"`
	Unix       int    `json:"unix"`
}

type ReleaseHistoryResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

// Lists Helm release history from the cluster.
func ReleaseHistory(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseHistoryOptions) (*ReleaseHistoryResultV1, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseHistoryOptionsDefaults(opts, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build release history options: %w", err)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace, // TODO: unset it everywhere
	})
	if err != nil {
		return nil, fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	loader.NoChartLockWarning = ""

	log.Default.Info(ctx, "Build release history")

	history, err := release.BuildHistory(releaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return nil, fmt.Errorf("build release history: %w", err)
	}

	result := &ReleaseHistoryResultV1{
		APIVersion: "v1",
	}

	releases := history.Releases()
	if len(releases) == 0 {
		return nil, &ReleaseNotFoundError{
			ReleaseName:      releaseName,
			ReleaseNamespace: releaseNamespace,
		}
	}

	for _, release := range releases {
		result.Releases = append(result.Releases, &ReleaseHistoryResultRelease{
			Annotations: release.Info.Annotations,
			Chart: &ReleaseHistoryResultChart{
				Name:       release.Chart.Name(),
				Version:    release.Chart.Metadata.Version,
				AppVersion: release.Chart.Metadata.AppVersion,
			},
			Description: release.Info.Description,
			Name:        release.Name,
			Namespace:   release.Namespace,
			Revision:    release.Version,
			Status:      release.Info.Status,
			Updated: &ReleaseHistoryResultUpdated{
				Human:      release.Info.LastDeployed.String(),
				HumanTable: release.Info.LastDeployed.Format(time.ANSIC),
				Unix:       int(release.Info.LastDeployed.Unix()),
			},
		})
	}

	sort.SliceStable(result.Releases, func(i, j int) bool {
		return result.Releases[i].Revision < result.Releases[j].Revision
	})

	if opts.Max > 0 && len(result.Releases) > opts.Max {
		result.Releases = result.Releases[len(result.Releases)-opts.Max:]
	}

	if opts.OutputNoPrint {
		return result, nil
	}

	var resultMessage string

	switch opts.OutputFormat {
	case common.OutputFormatTable:
		table := buildReleaseHistoryOutputTable(ctx, result)
		resultMessage = table.Render() + "\n"
	case common.OutputFormatJSON:
		b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
		if err != nil {
			return nil, fmt.Errorf("marshal result to json: %w", err)
		}

		resultMessage = string(b) + "\n"
	case common.OutputFormatYAML:
		b, err := yaml.MarshalContext(ctx, result, yaml.UseLiteralStyleIfMultiline(true))
		if err != nil {
			return nil, fmt.Errorf("marshal result to yaml: %w", err)
		}

		resultMessage = string(b)
	default:
		return nil, fmt.Errorf("unknown output format %q", opts.OutputFormat)
	}

	var colorLevel color.Level
	if color.Enable {
		colorLevel = color.TermColorLevel()
	}

	if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, opts.OutputFormat, colorLevel); err != nil {
		return nil, fmt.Errorf("write result to output: %w", err)
	}

	return result, nil
}

func buildReleaseHistoryOutputTable(ctx context.Context, result *ReleaseHistoryResultV1) prtable.Writer {
	table := prtable.NewWriter()
	setReleaseHistoryOutputTableStyle(ctx, table)

	headerRow := prtable.Row{
		color.New(color.Bold).Sprintf("REVISION"),
		color.New(color.Bold).Sprintf("UPDATED"),
		color.New(color.Bold).Sprintf("STATUS"),
		color.New(color.Bold).Sprintf("CHART"),
		color.New(color.Bold).Sprintf("APP VERSION"),
		color.New(color.Bold).Sprintf("DESCRIPTION"),
	}

	table.AppendHeader(headerRow)

	for _, release := range result.Releases {
		var statusColor color.Color
		switch release.Status {
		case helmreleasestatus.StatusDeployed, helmreleasestatus.StatusSuperseded:
			statusColor = color.Green
		case helmreleasestatus.StatusFailed:
			statusColor = color.LightRed
		default:
			statusColor = color.LightYellow
		}

		row := prtable.Row{
			release.Revision,
			release.Updated.HumanTable,
			color.New(statusColor).Sprint(release.Status),
			color.New(color.Cyan).Sprintf("%s-%s", release.Chart.Name, release.Chart.Version),
			release.Chart.AppVersion,
			release.Description,
		}

		table.AppendRow(row)
	}

	return table
}

func applyReleaseHistoryOptionsDefaults(opts ReleaseHistoryOptions, homeDir string) (ReleaseHistoryOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseHistoryOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultReleaseHistoryOutputFormat
	}

	return opts, nil
}

func setReleaseHistoryOutputTableStyle(ctx context.Context, table prtable.Writer) {
	style := prtable.StyleBoxDefault
	style.PaddingLeft = ""
	style.PaddingRight = "  "

	columnConfigs := []prtable.ColumnConfig{
		{
			Number: 1,
			Align:  text.AlignLeft,
		},
		{
			Number: 2,
			Align:  text.AlignLeft,
		},
		{
			Number: 3,
			Align:  text.AlignLeft,
		},
		{
			Number: 4,
			Align:  text.AlignLeft,
		},
		{
			Number: 5,
			Align:  text.AlignLeft,
		},
		{
			Number: 6,
			Align:  text.AlignLeft,
		},
	}

	tableWidth := log.Default.BlockContentWidth(ctx)
	if tableWidth < 20 {
		tableWidth = 140
	} else if tableWidth > 200 {
		tableWidth = 200
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))

	columnConfigs[0].WidthMax = 10
	columnConfigs[1].WidthMax = 25
	columnConfigs[2].WidthMax = 12
	columnConfigs[3].WidthMax = 24
	columnConfigs[4].WidthMax = 16
	columnConfigs[5].WidthMax = tableWidth - paddingsWidth - columnConfigs[0].WidthMax - columnConfigs[1].WidthMax - columnConfigs[2].WidthMax - columnConfigs[3].WidthMax - columnConfigs[4].WidthMax

	table.SetColumnConfigs(columnConfigs)
	table.SetStyle(prtable.Style{
		Box:     style,
		Color:   prtable.ColorOptionsDefault,
		Format:  prtable.FormatOptionsDefault,
		HTML:    prtable.DefaultHTMLOptions,
		Options: prtable.OptionsNoBordersAndSeparators,
		Title:   prtable.TitleOptionsDefault,
	})
	table.SuppressTrailingSpaces()
}
