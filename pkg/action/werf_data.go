package action

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/chart"
	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

type ResolveRecordedReleaseOptions struct{}

// ReleaseRequiredError is returned when the release name or namespace is neither given nor
// recorded in the chart's .werf-data.
type ReleaseRequiredError struct {
	MissingName      bool
	MissingNamespace bool
}

func (e *ReleaseRequiredError) Error() string {
	return fmt.Sprintf("%s required: not given and not recorded in the chart's %s", missingReleaseFields(e.MissingName, e.MissingNamespace), werfdata.DirName)
}

// ResolveRecordedRelease fills in an empty release name and namespace with the ones recorded in the
// .werf-data of the local chart at chartPath (see chart.DownloadChart for a remote one) and logs the
// ones it used. What is neither given nor recorded is returned empty.
func ResolveRecordedRelease(ctx context.Context, chartPath, releaseName, releaseNamespace string, opts ResolveRecordedReleaseOptions) (string, string, error) {
	if releaseName != "" && releaseNamespace != "" {
		return releaseName, releaseNamespace, nil
	}

	metadata, err := chart.LoadChartWerfDataMetadata(ctx, chartPath)
	if err != nil {
		return "", "", fmt.Errorf("load chart metadata: %w", err)
	}

	return resolveRecordedRelease(ctx, metadata.Release, releaseName, releaseNamespace)
}

// mergeWerfDataRuntimeOptions merges the annotations and labels recorded in the chart into opts. The
// caller's annotations and labels win over the chart's per key.
func mergeWerfDataRuntimeOptions(chart helmchart.Accessor, opts common.ReleaseInstallRuntimeOptions) (common.ReleaseInstallRuntimeOptions, error) {
	werfData, err := chartMetadataWerfData(chart)
	if err != nil {
		return common.ReleaseInstallRuntimeOptions{}, err
	}

	opts.ExtraAnnotations = mergeChartMetadataMap(werfData.Metadata.ExtraAnnotations, opts.ExtraAnnotations)
	opts.ExtraLabels = mergeChartMetadataMap(werfData.Metadata.ExtraLabels, opts.ExtraLabels)
	opts.ExtraRuntimeAnnotations = mergeChartMetadataMap(werfData.Metadata.ExtraRuntimeAnnotations, opts.ExtraRuntimeAnnotations)
	opts.ReleaseInfoAnnotations = mergeChartMetadataMap(werfData.Metadata.ReleaseInfoAnnotations, opts.ReleaseInfoAnnotations)

	return opts, nil
}

// chartMetadataWerfData is chartWerfData for applying the chart's annotations and labels: a chart
// without .werf-data published in the legacy bundle format gets the ones of its
// extra_annotations.json and extra_labels.json, as legacy bundles were applied with them. Only
// install, plan, render and lint read them, so a stored release's files never block rollback or
// uninstall.
func chartMetadataWerfData(chart helmchart.Accessor) (*werfdata.Data, error) {
	if chart == nil || len(chart.WerfDataFiles()) > 0 {
		return chartWerfData(chart)
	}

	data := &werfdata.Data{}
	if _, err := migrateLegacyExtraAnnotationsAndLabels(chart.Files(), data); err != nil {
		return nil, fmt.Errorf("read legacy annotations and labels of chart %q: %w", chart.Name(), err)
	}

	return data, nil
}

// chartWerfData returns the parsed .werf-data of the chart, empty if it has none. Stored releases
// never have .werf-data, so rollback and uninstall get an empty one.
func chartWerfData(chart helmchart.Accessor) (*werfdata.Data, error) {
	if chart == nil || len(chart.WerfDataFiles()) == 0 {
		return &werfdata.Data{}, nil
	}

	data, err := werfdata.Parse(chart.WerfDataFiles())
	if err != nil {
		return nil, fmt.Errorf("parse %s of chart %q: %w", werfdata.DirName, chart.Name(), err)
	}

	return data, nil
}

// loadRecordedRelease downloads a remote chart into downloadDir and returns its local path, to
// render instead of downloading it again, and the release name and namespace with the empty ones
// filled in from its .werf-data.
func loadRecordedRelease(ctx context.Context, chartPath string, registryClient *registry.Client, downloadDir, releaseName, releaseNamespace string, opts chart.DownloadChartOptions) (string, string, string, error) {
	chartPath, err := chart.DownloadChart(ctx, chartPath, registryClient, downloadDir, opts)
	if err != nil {
		return "", "", "", fmt.Errorf("download chart: %w", err)
	}

	releaseName, releaseNamespace, err = ResolveRecordedRelease(ctx, chartPath, releaseName, releaseNamespace, ResolveRecordedReleaseOptions{})
	if err != nil {
		return "", "", "", err
	}

	return chartPath, releaseName, releaseNamespace, nil
}

// mergeChartMetadataMap returns the entries of base overridden per key by the ones of overrides.
func mergeChartMetadataMap(base, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return overrides
	}

	return lo.Assign(base, overrides)
}

func missingReleaseFields(missingName, missingNamespace bool) string {
	var missing []string
	if missingName {
		missing = append(missing, "release name")
	}

	if missingNamespace {
		missing = append(missing, "release namespace")
	}

	return strings.Join(missing, " and ")
}

func requireRelease(releaseName, releaseNamespace string) error {
	if releaseName == "" || releaseNamespace == "" {
		return &ReleaseRequiredError{MissingName: releaseName == "", MissingNamespace: releaseNamespace == ""}
	}

	return nil
}

func resolveRecordedRelease(ctx context.Context, recorded *werfdata.ReleaseMetadata, releaseName, releaseNamespace string) (string, string, error) {
	if recorded == nil {
		return releaseName, releaseNamespace, nil
	}

	var used []string
	if releaseName == "" && recorded.Name != "" {
		releaseName = recorded.Name
		used = append(used, fmt.Sprintf("release name %q", recorded.Name))
	}

	if releaseNamespace == "" && recorded.Namespace != "" {
		releaseNamespace = recorded.Namespace
		used = append(used, fmt.Sprintf("release namespace %q", recorded.Namespace))
	}

	if len(used) > 0 {
		log.Default.Info(ctx, "Using %s recorded in %s", strings.Join(used, " and "), werfdata.DirName)
	}

	return releaseName, releaseNamespace, nil
}
