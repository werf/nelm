package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/chart/loader"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseListOutputFormat = common.OutputFormatTable
	DefaultReleaseListLogLevel     = log.ErrorLevel
)

type ReleaseListOptions struct {
	common.KubeConnectionOptions

	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// OutputFormat specifies the output format for the release list.
	// Valid values: "table" (default), "yaml", "json".
	// Defaults to DefaultReleaseListOutputFormat (table) if not specified.
	OutputFormat string
	// OutputNoPrint, when true, suppresses printing the output and only returns the result data structure.
	// Useful when calling this programmatically.
	OutputNoPrint bool
	// ReleaseNamespace specifies the namespace to list releases from.
	// If empty, uses the namespace from kubeconfig context.
	ReleaseNamespace string
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

// Lists Helm releases from the cluster.
func ReleaseList(ctx context.Context, opts ReleaseListOptions) (*ReleaseListResultV1, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseListOptionsDefaults(opts, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build release list options: %w", err)
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  opts.ReleaseNamespace, // TODO: unset it everywhere
	})
	if err != nil {
		return nil, fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, opts.ReleaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	loader.NoChartLockWarning = ""

	log.Default.Info(ctx, "Build release histories")

	histories, err := release.BuildHistories(releaseStorage, release.HistoryOptions{})
	if err != nil {
		return nil, fmt.Errorf("build release histories: %w", err)
	}

	result := &ReleaseListResultV1{
		APIVersion: "v1",
	}

	for _, history := range histories {
		releases := history.Releases()
		lastRelease := lo.LastOrEmpty(releases)

		result.Releases = append(result.Releases, &ReleaseListResultRelease{
			Name:      lastRelease.Name,
			Namespace: lastRelease.Namespace,
			Revision:  lastRelease.Version,
			Status:    lastRelease.Info.Status,
			DeployedAt: &ReleaseListResultDeployedAt{
				Human: time.Time{}.String(),
				Unix:  int(time.Time{}.Unix()),
			},
			Annotations: lastRelease.Info.Annotations,
			Chart: &ReleaseListResultChart{
				Name:       lastRelease.Chart.Name(),
				Version:    lastRelease.Chart.Metadata.Version,
				AppVersion: lastRelease.Chart.Metadata.AppVersion,
			},
		})
	}

	sort.SliceStable(result.Releases, func(i, j int) bool {
		if result.Releases[i].Namespace != result.Releases[j].Namespace {
			return result.Releases[i].Namespace < result.Releases[j].Namespace
		}

		return result.Releases[i].Name < result.Releases[j].Name
	})

	if opts.OutputNoPrint {
		return result, nil
	}

	var resultMessage string

	switch opts.OutputFormat {
	case common.OutputFormatTable:
		table := buildReleaseListOutputTable(ctx, result, opts.ReleaseNamespace != "")
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

func applyReleaseListOptionsDefaults(opts ReleaseListOptions, homeDir string) (ReleaseListOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseListOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultReleaseListOutputFormat
	}

	return opts, nil
}

type ReleaseListResultV1 struct {
	APIVersion string                      `json:"apiVersion"`
	Releases   []*ReleaseListResultRelease `json:"releases"`
}

type ReleaseListResultRelease struct {
	Name        string                       `json:"name"`
	Namespace   string                       `json:"namespace"`
	Revision    int                          `json:"revision"`
	Status      helmrelease.Status           `json:"status"`
	DeployedAt  *ReleaseListResultDeployedAt `json:"deployedAt"`
	Annotations map[string]string            `json:"annotations"`
	Chart       *ReleaseListResultChart      `json:"chart"`
}

// TODO(major): get rid
type ReleaseListResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseListResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

func buildReleaseListOutputTable(ctx context.Context, result *ReleaseListResultV1, namespaced bool) prtable.Writer {
	table := prtable.NewWriter()
	setReleaseListOutputTableStyle(ctx, table)

	headerRow := prtable.Row{
		color.New(color.Bold).Sprintf("NAME"),
		color.New(color.Bold).Sprintf("STATUS"),
		color.New(color.Bold).Sprintf("REVISION"),
	}
	if !namespaced {
		headerRow = append([]interface{}{color.New(color.Bold).Sprintf("NAMESPACE")}, headerRow...)
	}

	table.AppendHeader(headerRow)

	for _, release := range result.Releases {
		var statusColor color.Color
		switch release.Status {
		case helmrelease.StatusDeployed, helmrelease.StatusSuperseded:
			statusColor = color.Green
		case helmrelease.StatusFailed:
			statusColor = color.LightRed
		default:
			statusColor = color.LightYellow
		}

		row := prtable.Row{
			color.New(color.Cyan).Sprintf("%s", release.Name),
			color.New(statusColor).Sprintf("%s", string(release.Status)),
			release.Revision,
		}
		if !namespaced {
			row = append([]interface{}{release.Namespace}, row...)
		}

		table.AppendRow(row)
	}

	return table
}

func setReleaseListOutputTableStyle(ctx context.Context, table prtable.Writer) {
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
	}

	tableWidth := log.Default.BlockContentWidth(ctx)
	if tableWidth < 20 {
		tableWidth = 140
	} else if tableWidth > 200 {
		tableWidth = 200
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))
	columnsWidth := tableWidth - paddingsWidth

	columnConfigs[2].WidthMax = 16
	columnConfigs[3].WidthMax = 8
	columnConfigs[0].WidthMax = int(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax) * 0.3)
	columnConfigs[1].WidthMax = int(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax) * 0.4)
	columnConfigs[4].WidthMax = int(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax) * 0.4)

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
