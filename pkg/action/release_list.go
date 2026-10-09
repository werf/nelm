package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"k8s.io/apimachinery/pkg/labels"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
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

	// MetadataOnly lists releases by storage metadata without reading release bodies. The
	// detail getters of the returned releases then read each body on first use. Printing a
	// table never needs the bodies, so it always lists by metadata.
	MetadataOnly bool
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

type ReleaseListResultV2 struct {
	APIVersion string                      `json:"apiVersion"`
	Releases   []*ReleaseListResultRelease `json:"releases"`
}

// ReleaseListResultRelease is the latest revision of a release. Its fields come from storage
// metadata, while the getters return details of the stored release body: listed eagerly they
// are already decoded, listed with MetadataOnly the first getter call reads and decodes the
// body and later calls reuse it.
type ReleaseListResultRelease struct {
	Name      string                   `json:"name"`
	Namespace string                   `json:"namespace"`
	Revision  int                      `json:"revision"`
	Status    helmreleasestatus.Status `json:"status"`

	details *kdutil.Concurrent[*releaseListDetails]
}

func (r *ReleaseListResultRelease) Annotations(ctx context.Context) (map[string]string, error) {
	summary, err := r.loadSummary(ctx)
	if err != nil {
		return nil, err
	}

	return maps.Clone(summary.Annotations), nil
}

func (r *ReleaseListResultRelease) Chart(ctx context.Context) (*ReleaseListResultChart, error) {
	summary, err := r.loadSummary(ctx)
	if err != nil {
		return nil, err
	}

	if summary.Chart == nil {
		return nil, fmt.Errorf("release %q (namespace: %q, revision: %d) has no chart metadata", r.Name, r.Namespace, r.Revision)
	}

	return &ReleaseListResultChart{
		AppVersion: summary.Chart.AppVersion,
		Name:       summary.Chart.Name,
		Version:    summary.Chart.Version,
	}, nil
}

func (r *ReleaseListResultRelease) DeployedAt(ctx context.Context) (*ReleaseListResultDeployedAt, error) {
	summary, err := r.loadSummary(ctx)
	if err != nil {
		return nil, err
	}

	return &ReleaseListResultDeployedAt{
		Human: summary.DeployedAt.String(),
		Unix:  int(summary.DeployedAt.Unix()),
	}, nil
}

func (r *ReleaseListResultRelease) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(r.output())
	if err != nil {
		return nil, fmt.Errorf("marshal release %q: %w", r.Name, err)
	}

	return b, nil
}

func (r *ReleaseListResultRelease) MarshalYAML() (any, error) {
	return r.output(), nil
}

// loadSummary reads the body on first use. Concurrent calls share one read, and a caller
// waiting for it stops waiting when its own context ends. Undecodable bodies are remembered,
// while other errors, such as a failed request, a canceled reader or a revision pruned since
// the listing, are retried by the next call.
func (r *ReleaseListResultRelease) loadSummary(ctx context.Context) (*release.ReleaseSummary, error) {
	if r.details == nil {
		return nil, fmt.Errorf("release %q (namespace: %q) was not returned by ReleaseList", r.Name, r.Namespace)
	}

	for {
		var (
			summary  *release.ReleaseSummary
			err      error
			loading  chan struct{}
			storage  release.ReleaseStorager
			revision release.Revision
		)

		r.details.RWTransaction(func(details *releaseListDetails) {
			switch {
			case details.summary != nil:
				summary = details.summary
			case details.err != nil:
				err = details.err
			case details.storage == nil:
				err = errors.New("no details")
			case details.loading != nil:
				loading = details.loading
			default:
				details.loading = make(chan struct{})
				storage, revision = details.storage, details.revision
			}
		})

		if storage != nil {
			err = r.readSummary(ctx, storage, revision)
		}

		switch {
		case summary != nil:
			return summary, nil
		case err != nil:
			return nil, fmt.Errorf("read details of release %q (namespace: %q, revision: %d): %w", r.Name, r.Namespace, r.Revision, err)
		case loading == nil:
			continue
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for details of release %q (namespace: %q, revision: %d): %w", r.Name, r.Namespace, r.Revision, ctx.Err())
		case <-loading:
		}
	}
}

// output holds the details already read, without reading bodies, since marshaling has no
// context to read them with.
func (r *ReleaseListResultRelease) output() releaseListResultReleaseOutput {
	output := releaseListResultReleaseOutput{
		Name:      r.Name,
		Namespace: r.Namespace,
		Revision:  r.Revision,
		Status:    r.Status,
	}

	if r.details == nil {
		return output
	}

	r.details.RTransaction(func(details *releaseListDetails) {
		if details.summary == nil {
			return
		}

		output.Annotations = details.summary.Annotations
		output.DeployedAt = &ReleaseListResultDeployedAt{
			Human: details.summary.DeployedAt.String(),
			Unix:  int(details.summary.DeployedAt.Unix()),
		}

		if details.summary.Chart != nil {
			output.Chart = &ReleaseListResultChart{
				AppVersion: details.summary.Chart.AppVersion,
				Name:       details.summary.Chart.Name,
				Version:    details.summary.Chart.Version,
			}
		}
	})

	return output
}

// readSummary reads the body outside the lock, remembers a decoded or undecodable result and
// wakes the callers waiting for it. It returns the error of a read that is not remembered.
func (r *ReleaseListResultRelease) readSummary(ctx context.Context, storage release.ReleaseStorager, revision release.Revision) error {
	defer r.details.RWTransaction(func(details *releaseListDetails) {
		close(details.loading)
		details.loading = nil
	})

	loaded, err := storage.LoadRevisionSummary(ctx, revision)
	if err != nil && !errors.Is(err, release.ErrReleaseUndecodable) {
		return fmt.Errorf("load stored release: %w", err)
	}

	if err == nil && loaded == nil {
		err = errors.New("no details")
	}

	r.details.RWTransaction(func(details *releaseListDetails) {
		details.summary, details.err = loaded, err
	})

	return nil
}

type releaseListDetails struct {
	err      error
	loading  chan struct{}
	revision release.Revision
	storage  release.ReleaseStorager
	summary  *release.ReleaseSummary
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

type releaseListResultReleaseOutput struct {
	Name        string                       `json:"name"`
	Namespace   string                       `json:"namespace"`
	Revision    int                          `json:"revision"`
	Status      helmreleasestatus.Status     `json:"status"`
	DeployedAt  *ReleaseListResultDeployedAt `json:"deployedAt"`
	Annotations map[string]string            `json:"annotations"`
	Chart       *ReleaseListResultChart      `json:"chart"`
}

func ReleaseList(ctx context.Context, opts ReleaseListOptions) (*ReleaseListResultV2, error) {
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

	var result *ReleaseListResultV2
	if listsReleasesByMetadata(opts) {
		result, err = buildReleaseListResultFromMetadata(ctx, releaseStorage, opts.ReleaseLabelSelector)
	} else {
		result, err = buildReleaseListResult(ctx, releaseStorage, opts.ReleaseLabelSelector)
	}

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

func buildReleaseListOutputTable(ctx context.Context, result *ReleaseListResultV2, namespaced bool) prtable.Writer {
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

func buildReleaseListResult(ctx context.Context, storage release.ReleaseStorager, selector string) (*ReleaseListResultV2, error) {
	summaries, err := storage.ListLatestSummaries(ctx, release.ListLatestSummariesOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("read release list: %w", err)
	}

	releases := make([]*ReleaseListResultRelease, 0, len(summaries))
	for _, summary := range summaries {
		if summary.DecodeErr != nil {
			log.Default.Error(ctx, "Showing release %q (namespace: %q, revision: %d) without its details: %s", summary.Revision.Name, summary.Revision.Namespace, summary.Revision.Version, summary.DecodeErr)
		}

		releases = append(releases, &ReleaseListResultRelease{
			Name:      summary.Revision.Name,
			Namespace: summary.Revision.Namespace,
			Revision:  summary.Revision.Version,
			Status:    helmreleasestatus.Status(summary.Revision.Status),
			details:   kdutil.NewConcurrent(&releaseListDetails{err: summary.DecodeErr, revision: summary.Revision, summary: summary.Summary}),
		})
	}

	return newReleaseListResult(releases), nil
}

func buildReleaseListResultFromMetadata(ctx context.Context, storage release.ReleaseStorager, selector string) (*ReleaseListResultV2, error) {
	revisions, err := storage.LatestRevisions(ctx, release.LatestRevisionsOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("read release list: %w", err)
	}

	releases := make([]*ReleaseListResultRelease, 0, len(revisions))
	for _, revision := range revisions {
		releases = append(releases, &ReleaseListResultRelease{
			Name:      revision.Name,
			Namespace: revision.Namespace,
			Revision:  revision.Version,
			Status:    helmreleasestatus.Status(revision.Status),
			details:   kdutil.NewConcurrent(&releaseListDetails{revision: revision, storage: storage}),
		})
	}

	return newReleaseListResult(releases), nil
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

func listsReleasesByMetadata(opts ReleaseListOptions) bool {
	return opts.MetadataOnly || (!opts.OutputNoPrint && opts.OutputFormat == common.OutputFormatTable)
}

func newReleaseListResult(releases []*ReleaseListResultRelease) *ReleaseListResultV2 {
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Namespace != releases[j].Namespace {
			return releases[i].Namespace < releases[j].Namespace
		}

		return releases[i].Name < releases[j].Name
	})

	return &ReleaseListResultV2{
		APIVersion: "v2",
		Releases:   releases,
	}
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
