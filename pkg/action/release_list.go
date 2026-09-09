package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	"github.com/werf/nelm/pkg/helm/pkg/storage/driver"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/release"
)

const (
	DefaultReleaseListLogLevel     = log.ErrorLevel
	DefaultReleaseListOutputFormat = common.OutputFormatTable
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

func newReleaseListResultRelease(rel *helmrelease.Release) *ReleaseListResultRelease {
	result := &ReleaseListResultRelease{
		Name:      rel.Name,
		Namespace: rel.Namespace,
		Revision:  rel.Version,
	}

	if rel.Info != nil {
		result.Annotations = rel.Info.Annotations
		result.Status = rel.Info.Status
		result.DeployedAt = &ReleaseListResultDeployedAt{
			Human: rel.Info.LastDeployed.String(),
			Unix:  int(rel.Info.LastDeployed.Unix()),
		}
	}

	if rel.Chart != nil && rel.Chart.Metadata != nil {
		result.Chart = &ReleaseListResultChart{
			AppVersion: rel.Chart.Metadata.AppVersion,
			Name:       rel.Chart.Metadata.Name,
			Version:    rel.Chart.Metadata.Version,
		}
	}

	return result
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

	log.Default.Info(ctx, "List releases")

	metas, err := releaseStorage.ListReleaseMeta()
	if err != nil {
		return nil, fmt.Errorf("list release metadata: %w", err)
	}

	releases, err := buildReleaseListResultReleases(ctx, releaseStorage, lastReleaseRevisions(metas), opts.NetworkParallelism)
	if err != nil {
		return nil, err
	}

	result := &ReleaseListResultV1{
		APIVersion: "v1",
		Releases:   releases,
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

func buildReleaseListResultReleases(ctx context.Context, releaseStorage *helmstorage.Storage, metas []driver.ReleaseMeta, networkParallelism int) ([]*ReleaseListResultRelease, error) {
	releasesPool := pool.NewWithResults[[]*ReleaseListResultRelease]().WithContext(ctx).WithMaxGoroutines(networkParallelism).WithCancelOnError().WithFirstError()

	for _, meta := range metas {
		releasesPool.Go(func(_ context.Context) ([]*ReleaseListResultRelease, error) {
			rel, err := getReleaseRevision(releaseStorage, meta)
			if err != nil {
				// The revision was trimmed from the history between listing metadata and
				// fetching it, so the release is no longer part of the listing.
				if errors.Is(err, driver.ErrReleaseNotFound) {
					return nil, nil
				}

				return nil, fmt.Errorf("get release %q (namespace: %q, revision: %d): %w", meta.Name, meta.Namespace, meta.Version, err)
			}

			return []*ReleaseListResultRelease{newReleaseListResultRelease(rel)}, nil
		})
	}

	releases, err := releasesPool.Wait()
	if err != nil {
		return nil, fmt.Errorf("wait for release pool: %w", err)
	}

	return lo.Flatten(releases), nil
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

func getReleaseRevision(releaseStorage *helmstorage.Storage, meta driver.ReleaseMeta) (*helmrelease.Release, error) {
	rels, err := releaseStorage.Query(map[string]string{
		"name":    meta.Name,
		"owner":   "helm",
		"version": strconv.Itoa(meta.Version),
	})
	if err != nil {
		return nil, fmt.Errorf("query release revision: %w", err)
	}

	if rel, found := lo.Find(rels, func(rel *helmrelease.Release) bool {
		return rel.Namespace == meta.Namespace
	}); found {
		return rel, nil
	}

	// Releases stored by ancient Helm versions have no namespace in the release body,
	// so there is nothing to match the metadata namespace against.
	if len(rels) == 1 {
		return rels[0], nil
	}

	return nil, driver.ErrReleaseNotFound
}

func lastReleaseRevisions(metas []driver.ReleaseMeta) []driver.ReleaseMeta {
	latest := make(map[string]driver.ReleaseMeta, len(metas))

	for _, meta := range metas {
		key := meta.Namespace + "/" + meta.Name

		if current, found := latest[key]; found && current.Version >= meta.Version {
			continue
		}

		latest[key] = meta
	}

	return lo.Values(latest)
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
