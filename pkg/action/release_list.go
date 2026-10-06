package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/release"
)

const (
	DefaultReleaseListLogLevel     = log.ErrorLevel
	DefaultReleaseListOutputFormat = common.OutputFormatTable
)

type ReleaseListOptions struct {
	common.KubeConnectionOptions

	// NetworkParallelism is retained for compatibility and is not used by release listing.
	NetworkParallelism int
	// OutputFormat specifies the output format for the release list.
	// Valid values: "table" (default), "yaml", "json".
	// Defaults to DefaultReleaseListOutputFormat (table) if not specified.
	OutputFormat string
	// OutputNoPrint, when true, suppresses printing the output and only returns the result data structure.
	// Useful when calling this programmatically.
	OutputNoPrint bool
	// ReleaseLabelSelector filters storage objects using Kubernetes label selector syntax.
	// The result contains the latest matching revision, not necessarily the latest overall.
	ReleaseLabelSelector string
	// ReleaseNamespace specifies the namespace to list releases from.
	// If empty, lists all namespaces.
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

type ReleaseListResultV1 struct {
	APIVersion string                      `json:"apiVersion"`
	Releases   []*ReleaseListResultRelease `json:"releases"`
}

type ReleaseListResultRelease struct {
	Name        string                       `json:"name"`
	Namespace   string                       `json:"namespace"`
	Revision    int                          `json:"revision"`
	Status      helmreleasestatus.Status     `json:"status"`
	DeployedAt  *ReleaseListResultDeployedAt `json:"deployedAt"`
	Annotations map[string]string            `json:"annotations"`
	Chart       *ReleaseListResultChart      `json:"chart"`
}

type ReleaseListResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseListResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

func ReleaseList(ctx context.Context, opts ReleaseListOptions) (*ReleaseListResultV1, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseListOptionsDefaults(opts, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build release list options: %w", err)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, kube.KubeConfigOptions{
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

	log.Default.Info(ctx, "List releases")

	result, err := buildReleaseListResult(ctx, releaseStorage, opts.ReleaseLabelSelector)
	if err != nil {
		return nil, fmt.Errorf("list latest releases: %w", err)
	}

	if opts.OutputNoPrint {
		return result, nil
	}

	var resultMessage string

	switch opts.OutputFormat {
	case common.OutputFormatTable:
		table := buildReleaseListOutputTable(ctx, result, opts.ReleaseNamespace != "")
		resultMessage = table.Render() + "\n"
	case common.OutputFormatJSON, common.OutputFormatYAML:
		if opts.OutputFormat == common.OutputFormatJSON {
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b) + "\n"
		} else {
			b, err := yaml.MarshalContext(ctx, result, yaml.UseLiteralStyleIfMultiline(true))
			if err != nil {
				return nil, fmt.Errorf("marshal result to yaml: %w", err)
			}

			resultMessage = string(b)
		}
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
		case helmreleasestatus.StatusDeployed, helmreleasestatus.StatusSuperseded:
			statusColor = color.Green
		case helmreleasestatus.StatusFailed:
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

func applyReleaseListOptionsDefaults(opts ReleaseListOptions, homeDir string) (ReleaseListOptions, error) {
	if _, err := labels.Parse(opts.ReleaseLabelSelector); err != nil {
		return ReleaseListOptions{}, fmt.Errorf("parse release label selector: %w", err)
	}

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

func buildReleaseListResult(ctx context.Context, storage release.ReleaseStorager, selector string) (*ReleaseListResultV1, error) {
	latest := map[string]*ReleaseListResultRelease{}
	readErrors := map[string]error{}

	if err := storage.ForEachLatestRelease(ctx, func(revision release.Revision, rel helmrel.Accessor, readErr error) error {
		id := revision.Namespace + "/" + revision.Name
		if current, found := latest[id]; found && current.Revision >= revision.Version {
			return nil
		}
		entry := &ReleaseListResultRelease{
			Name: revision.Name, Namespace: revision.Namespace, Revision: revision.Version,
			Status: helmreleasestatus.Status(revision.Status),
		}
		latest[id] = entry
		delete(readErrors, id)
		if readErr != nil {
			readErrors[id] = readErr

			return nil
		}

		if lo.IsNil(rel.Chart()) {
			readErrors[id] = errors.New("chart is required")

			return nil
		}

		chartAccessor, err := helmchart.NewAccessor(rel.Chart())
		if err != nil {
			readErrors[id] = fmt.Errorf("construct chart accessor: %w", err)

			return nil
		}
		metadata := chartAccessor.MetadataAsMap()
		if metadata == nil {
			readErrors[id] = errors.New("chart metadata is required")

			return nil
		}
		version, _ := metadata["Version"].(string)
		appVersion, _ := metadata["AppVersion"].(string)
		entry.Annotations = rel.Annotations()
		entry.Chart = &ReleaseListResultChart{AppVersion: appVersion, Name: chartAccessor.Name(), Version: version}
		entry.DeployedAt = &ReleaseListResultDeployedAt{Human: rel.DeployedAt().String(), Unix: int(rel.DeployedAt().Unix())}

		return nil
	}, release.ForEachLatestReleaseOptions{LabelSelector: selector}); err != nil {
		return nil, fmt.Errorf("read release list: %w", err)
	}

	result := &ReleaseListResultV1{APIVersion: "v1"}
	for _, entry := range latest {
		result.Releases = append(result.Releases, entry)
	}

	sort.Slice(result.Releases, func(i, j int) bool {
		if result.Releases[i].Namespace != result.Releases[j].Namespace {
			return result.Releases[i].Namespace < result.Releases[j].Namespace
		}

		return result.Releases[i].Name < result.Releases[j].Name
	})

	for _, entry := range result.Releases {
		if err := readErrors[entry.Namespace+"/"+entry.Name]; err != nil {
			if !errors.Is(err, release.ErrReleaseUndecodable) {
				return nil, fmt.Errorf("read details of release %q (namespace: %q): %w", entry.Name, entry.Namespace, err)
			}

			log.Default.Error(ctx, "Showing release %q (namespace: %q, revision: %d) without its details: %s", entry.Name, entry.Namespace, entry.Revision, err)
		}
	}

	return result, nil
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
