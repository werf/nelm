package action

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/3p-helm/pkg/chart/loader"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/logboek"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
)

const (
	DefaultReleaseListOutputFormat = TableOutputFormat
	DefaultReleaseListLogLevel     = ErrorLogLevel
)

type ReleaseListOptions struct {
	KubeAPIServerName    string
	KubeBurstLimit       int
	KubeCAPath           string
	KubeConfigBase64     string
	KubeConfigPaths      []string
	KubeContext          string
	KubeQPSLimit         int
	KubeSkipTLSVerify    bool
	KubeTLSServerName    string
	KubeToken            string
	ReleaseNamespace     string
	NetworkParallelism   int
	OutputFormat         string
	OutputNoPrint        bool
	ReleaseStorageDriver string
	SQLConnectionString  string
	TempDirPath          string
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

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	// TODO(ilya-lesikov): some options are not propagated from cli/actions
	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		BurstLimit:            opts.KubeBurstLimit,
		CertificateAuthority:  opts.KubeCAPath,
		CurrentContext:        opts.KubeContext,
		InsecureSkipTLSVerify: opts.KubeSkipTLSVerify,
		KubeConfigBase64:      opts.KubeConfigBase64,
		Namespace:             opts.ReleaseNamespace,
		QPSLimit:              opts.KubeQPSLimit,
		Server:                opts.KubeAPIServerName,
		TLSServerName:         opts.KubeTLSServerName,
		Token:                 opts.KubeToken,
	})
	if err != nil {
		return nil, fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(
		ctx,
		opts.ReleaseNamespace,
		opts.ReleaseStorageDriver,
		release.ReleaseStorageOptions{
			StaticClient:        clientFactory.Static().(*kubernetes.Clientset),
			SQLConnectionString: opts.SQLConnectionString,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	loader.NoChartLockWarning = ""

	histories, err := release.BuildHistories(releaseStorage, release.BuildHistoriesOptions{
		DiscoveryClient: clientFactory.Discovery(),
		Mapper:          clientFactory.Mapper(),
	})
	if err != nil {
		return nil, fmt.Errorf("build release histories: %w", err)
	}

	result := &ReleaseListResultV1{
		ApiVersion: ReleaseListResultApiVersionV1,
	}

	for _, history := range histories {
		lastRelease, found, err := history.LastRelease()
		if err != nil {
			return nil, fmt.Errorf("get last release: %w", err)
		}

		if !found {
			continue
		}

		result.Releases = append(result.Releases, &ReleaseListResultRelease{
			Name:      lastRelease.Name(),
			Namespace: lastRelease.Namespace(),
			Revision:  lastRelease.Revision(),
			Status:    lastRelease.Status(),
			DeployedAt: &ReleaseListResultDeployedAt{
				Human: lastRelease.LastDeployed().String(),
				Unix:  int(lastRelease.LastDeployed().Unix()),
			},
			Annotations: lastRelease.InfoAnnotations(),
			Chart: &ReleaseListResultChart{
				Name:       lastRelease.ChartName(),
				Version:    lastRelease.ChartVersion(),
				AppVersion: lastRelease.AppVersion(),
			},
		})
	}

	sort.SliceStable(result.Releases, func(i, j int) bool {
		if result.Releases[i].Namespace != result.Releases[j].Namespace {
			return result.Releases[i].Namespace < result.Releases[j].Namespace
		}

		return result.Releases[i].Name < result.Releases[j].Name
	})

	if !opts.OutputNoPrint {
		var resultMessage string

		switch opts.OutputFormat {
		case TableOutputFormat:
			table := buildReleaseListOutputTable(ctx, result)
			resultMessage = table.Render()
		case JsonOutputFormat:
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b)
		case YamlOutputFormat:
			b, err := yaml.MarshalContext(ctx, result)
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

		if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, string(opts.OutputFormat), colorLevel); err != nil {
			return nil, fmt.Errorf("write result to output: %w", err)
		}
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

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = DefaultNetworkParallelism
	}

	if opts.KubeQPSLimit <= 0 {
		opts.KubeQPSLimit = DefaultQPSLimit
	}

	if opts.KubeBurstLimit <= 0 {
		opts.KubeBurstLimit = DefaultBurstLimit
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultReleaseListOutputFormat
	}

	return opts, nil
}

const ReleaseListResultApiVersionV1 = "v1"

type ReleaseListResultV1 struct {
	ApiVersion string                      `json:"apiVersion"`
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

type ReleaseListResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseListResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

func buildReleaseListOutputTable(ctx context.Context, result *ReleaseListResultV1) prtable.Writer {
	table := prtable.NewWriter()
	setReleaseListOutputTableStyle(ctx, table)

	table.AppendHeader(prtable.Row{
		color.New(color.Bold).Sprintf("NAMESPACE"),
		color.New(color.Bold).Sprintf("NAME"),
		color.New(color.Bold).Sprintf("STATUS"),
		color.New(color.Bold).Sprintf("REVISION"),
		color.New(color.Bold).Sprintf("DEPLOYED"),
	})

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

		table.AppendRow(prtable.Row{
			release.Namespace,
			color.New(color.Cyan).Sprintf(release.Name),
			color.New(statusColor).Sprintf(string(release.Status)),
			release.Revision,
			release.DeployedAt.Human,
		})
	}

	return table
}

func setReleaseListOutputTableStyle(ctx context.Context, table prtable.Writer) {
	style := prtable.StyleBoxDefault
	style.PaddingLeft = " "
	style.PaddingRight = " "

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

	tableWidth := logboek.Context(ctx).Streams().ContentWidth()
	if tableWidth < 20 {
		tableWidth = 140
	} else if tableWidth > 200 {
		tableWidth = 200
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))
	columnsWidth := tableWidth - paddingsWidth

	columnConfigs[2].WidthMax = 16
	columnConfigs[3].WidthMax = 8
	columnConfigs[0].WidthMax = int(math.Floor(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax)) * 0.3)
	columnConfigs[1].WidthMax = int(math.Floor(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax)) * 0.4)
	columnConfigs[4].WidthMax = int(math.Floor(float64(columnsWidth-columnConfigs[2].WidthMax-columnConfigs[3].WidthMax)) * 0.4)

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
