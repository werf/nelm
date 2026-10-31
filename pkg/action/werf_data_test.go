package action

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
)

var (
	werfDataCallerAnnotations        = map[string]string{"shared-annotation": "caller", "caller-annotation": "caller"}
	werfDataCallerLabels             = map[string]string{"shared-label": "caller", "caller-label": "caller"}
	werfDataCallerRuntimeAnnotations = map[string]string{"shared-runtime-annotation": "caller", "caller-runtime-annotation": "caller"}
	werfDataCallerInfoAnnotations    = map[string]string{"shared-info": "caller", "caller-info": "caller"}
	werfDataMergedAnnotations        = map[string]string{
		"chart-annotation": "chart", "shared-annotation": "caller", "caller-annotation": "caller",
	}
	werfDataMergedLabels = map[string]string{
		"chart-label": "chart", "shared-label": "caller", "caller-label": "caller",
	}
	werfDataMergedRuntimeAnnotations = map[string]string{
		"chart-runtime-annotation": "chart", "shared-runtime-annotation": "caller", "caller-runtime-annotation": "caller",
	}
	werfDataMergedInfoAnnotations = map[string]string{
		"chart-info": "chart", "shared-info": "caller", "caller-info": "caller",
	}
)

func TestChartLint_SeesWerfDataAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
	}{
		{name: "extra annotations", metadata: "apiVersion: v2\nextraAnnotations:\n  werf.io/weight: not-a-number\n"},
		{name: "extra runtime annotations", metadata: "apiVersion: v2\nextraRuntimeAnnotations:\n  werf.io/weight: not-a-number\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath := newWerfDataChart(t, tt.metadata)

			err := ChartLint(newChartRenderInstallContext(t), ChartLintOptions{
				Chart:       chartPath,
				TempDirPath: t.TempDir(),
			})
			require.ErrorContains(t, err, "validate weight")
		})
	}
}

func TestChartRender_FailsOnUnsupportedWerfDataAPIVersion(t *testing.T) {
	chartPath := newWerfDataChart(t, "apiVersion: v3\n")

	_, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
		Chart:         chartPath,
		OutputNoPrint: true,
		TempDirPath:   t.TempDir(),
	})
	require.ErrorContains(t, err, `unsupported .werf-data/metadata.yaml apiVersion "v3"`)
}

func TestChartRender_MergesWerfDataMetadataBelowCallerOptions(t *testing.T) {
	chartPath := newWerfDataChart(t, readWerfDataTestMetadata(t))

	result, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
		Chart:                   chartPath,
		ExtraAnnotations:        werfDataCallerAnnotations,
		ExtraLabels:             werfDataCallerLabels,
		ExtraRuntimeAnnotations: werfDataCallerRuntimeAnnotations,
		OutputNoPrint:           true,
		TempDirPath:             t.TempDir(),
	})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)

	annotations := result.Resources[0].Unstruct.GetAnnotations()
	for key, value := range werfDataMergedAnnotations {
		assert.Equal(t, value, annotations[key], key)
	}

	for key, value := range werfDataMergedRuntimeAnnotations {
		assert.Equal(t, value, annotations[key], key)
	}

	labels := result.Resources[0].Unstruct.GetLabels()
	for key, value := range werfDataMergedLabels {
		assert.Equal(t, value, labels[key], key)
	}
}

func TestReleaseInstall_AutoRollbackIgnoresWerfDataMetadata(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevision(t, api, 1, addProgressPhasesConfigMap(t, api))

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
		s.failConfigMaps = map[string]bool{"sh.helm.release.v1.progress-release.v2": true}
	})

	installOpts := newChartRenderInstallOptions(t, api, newWerfDataChart(t, readWerfDataTestMetadata(t)), nil)
	installOpts.AutoRollback = true
	installOpts.ExtraRuntimeAnnotations = werfDataCallerRuntimeAnnotations
	installOpts.LegacyHelmCompatibleTracking = true

	_, err := ReleaseInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", installOpts)
	require.ErrorContains(t, err, "update release")

	var applied []map[string]string
	api.state.RTransaction(func(s *progressPhasesAPIState) {
		for _, cm := range s.appliedConfigMaps {
			if cm.Name == "progress-resource" {
				applied = append(applied, cm.GetAnnotations())
			}
		}
	})

	require.Len(t, applied, 2, "applied by the install and by the rollback")
	assert.Equal(t, "chart", applied[0]["chart-runtime-annotation"])

	for key, value := range werfDataCallerRuntimeAnnotations {
		assert.Equal(t, value, applied[1][key], key)
	}

	assert.NotContains(t, applied[1], "chart-runtime-annotation")
	assert.NotContains(t, applied[1], "chart-annotation")
}

func TestReleaseInstall_MergesWerfDataMetadataBelowCallerOptions(t *testing.T) {
	tests := []struct {
		name              string
		chartRenderResult bool
	}{
		{name: "chart"},
		{name: "chart render result", chartRenderResult: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			chartPath := newWerfDataChart(t, readWerfDataTestMetadata(t))

			setWerfDataMergedClusterConfigMap(api)

			ctx := newChartRenderInstallContext(t)

			var renderResult *ChartRenderResultV2
			if tt.chartRenderResult {
				renderOpts := newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release")
				renderOpts.ExtraAnnotations = werfDataCallerAnnotations
				renderOpts.ExtraLabels = werfDataCallerLabels

				var err error

				renderResult, err = ChartRender(ctx, renderOpts)
				require.NoError(t, err)
			}

			installOpts := newChartRenderInstallOptions(t, api, chartPath, renderResult)
			installOpts.ExtraAnnotations = werfDataCallerAnnotations
			installOpts.ExtraLabels = werfDataCallerLabels
			installOpts.ExtraRuntimeAnnotations = werfDataCallerRuntimeAnnotations
			installOpts.ReleaseInfoAnnotations = werfDataCallerInfoAnnotations

			_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", installOpts)
			require.NoError(t, err)

			getResult, err := ReleaseGet(ctx, "progress-release", "progress-ns", ReleaseGetOptions{
				KubeConnectionOptions: api.kubeOpts,
				OutputNoPrint:         true,
				ReleaseStorageDriver:  "configmap",
				TempDirPath:           t.TempDir(),
			})
			require.NoError(t, err)

			for key, value := range werfDataMergedInfoAnnotations {
				assert.Equal(t, value, getResult.Release.Annotations[key], key)
			}

			require.Len(t, getResult.ResourceSpecs, 1)

			annotations := getResult.ResourceSpecs[0].Unstruct.GetAnnotations()
			for key, value := range werfDataMergedAnnotations {
				assert.Equal(t, value, annotations[key], key)
			}

			labels := getResult.ResourceSpecs[0].Unstruct.GetLabels()
			for key, value := range werfDataMergedLabels {
				assert.Equal(t, value, labels[key], key)
			}
		})
	}
}

func TestReleasePlanInstall_PlanArtifactCarriesWerfDataMetadata(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhasesConfigMap(t, api)
	chartPath := newWerfDataChart(t, readWerfDataTestMetadata(t))
	planArtifactPath := filepath.Join(t.TempDir(), "plan.gz")
	ctx := newChartRenderInstallContext(t)

	_, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		Chart:                 chartPath,
		PlanArtifactPath:      planArtifactPath,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
			ExtraAnnotations:          werfDataCallerAnnotations,
			ExtraLabels:               werfDataCallerLabels,
			ExtraRuntimeAnnotations:   werfDataCallerRuntimeAnnotations,
			ReleaseInfoAnnotations:    werfDataCallerInfoAnnotations,
			ReleaseStorageDriver:      "configmap",
			ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		},
		TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)

	planArtifact, err := ReadPlanArtifact(ctx, planArtifactPath, "", "")
	require.NoError(t, err)

	options := planArtifact.Data.Options
	assert.Equal(t, werfDataMergedAnnotations, options.ExtraAnnotations)
	assert.Equal(t, werfDataMergedLabels, options.ExtraLabels)
	assert.Equal(t, werfDataMergedRuntimeAnnotations, options.ExtraRuntimeAnnotations)
	assert.Equal(t, werfDataMergedInfoAnnotations, options.ReleaseInfoAnnotations)
}
