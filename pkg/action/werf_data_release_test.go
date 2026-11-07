package action

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const (
	recordedReleaseConfigMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: release
data:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
`
	werfDataRecordedReleaseMetadata = "apiVersion: v2\nrelease:\n  name: progress-release\n  namespace: progress-ns\n"
)

var (
	// chartAnnotationsTestCases are the annotations and labels that a chart, a legacy bundle or one
	// with .werf-data, and the caller set under the same keys, and the ones that win.
	chartAnnotationsTestCases = []struct {
		name     string
		werfData bool
	}{
		{name: "legacy bundle"},
		{name: ".werf-data", werfData: true},
	}
	chartAnnotationsTestCallerAnnotations        = map[string]string{"shared-annotation": "caller"}
	chartAnnotationsTestCallerLabels             = map[string]string{"shared-label": "caller"}
	chartAnnotationsTestCallerRuntimeAnnotations = map[string]string{"project.werf.io/name": "caller", "ci.werf.io/commit": "caller", "caller-runtime": "caller"}
	chartAnnotationsTestWantAnnotations          = map[string]string{"user-annotation": "chart", "shared-annotation": "caller"}
	chartAnnotationsTestWantLabels               = map[string]string{"user-label": "chart", "shared-label": "caller"}
	chartAnnotationsTestWantRuntimeAnnotations   = map[string]string{"project.werf.io/name": "caller", "ci.werf.io/commit": "caller", "werf.io/release-channel": "chart", "caller-runtime": "caller"}
)

func TestChartRender_ChartAnnotations(t *testing.T) {
	for _, tt := range chartAnnotationsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
				Chart:                   newChartAnnotationsTestChart(t, tt.werfData),
				ExtraAnnotations:        chartAnnotationsTestCallerAnnotations,
				ExtraLabels:             chartAnnotationsTestCallerLabels,
				ExtraRuntimeAnnotations: chartAnnotationsTestCallerRuntimeAnnotations,
				OutputNoPrint:           true,
				TempDirPath:             t.TempDir(),
			})
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)

			assertChartAnnotationsTestMetadata(t, result.Resources[0].Unstruct.GetAnnotations(), lo.Assign(chartAnnotationsTestWantAnnotations, chartAnnotationsTestWantRuntimeAnnotations))
			assertChartAnnotationsTestMetadata(t, result.Resources[0].Unstruct.GetLabels(), chartAnnotationsTestWantLabels)
		})
	}
}

func TestChartRender_LegacyBundleAnnotationsAndLabels(t *testing.T) {
	const (
		legacyAnnotations = `{"user-annotation": "chart", "shared-annotation": "chart", "project.werf.io/name": "app", "werf.io/release-channel": "stable"}`
		legacyLabels      = `{"user-label": "chart", "shared-label": "chart"}`
	)

	tests := []struct {
		name            string
		werfData        bool
		wantAnnotations map[string]string
		wantLabels      map[string]string
	}{
		{
			name:            "legacy bundle",
			wantAnnotations: map[string]string{"user-annotation": "chart", "shared-annotation": "caller", "project.werf.io/name": "app", "werf.io/release-channel": "stable"},
			wantLabels:      map[string]string{"user-label": "chart", "shared-label": "caller"},
		},
		{
			name:            "chart with .werf-data ignores root extra json files",
			werfData:        true,
			wantAnnotations: map[string]string{"shared-annotation": "caller"},
			wantLabels:      map[string]string{"shared-label": "caller"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath := newRecordedReleaseChart(t, "apiVersion: v2\n")
			if !tt.werfData {
				require.NoError(t, os.RemoveAll(filepath.Join(chartPath, werfdata.DirName)))
			}

			require.NoError(t, os.WriteFile(filepath.Join(chartPath, "extra_annotations.json"), []byte(legacyAnnotations), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(chartPath, "extra_labels.json"), []byte(legacyLabels), 0o600))

			result, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
				Chart:            chartPath,
				ExtraAnnotations: map[string]string{"shared-annotation": "caller"},
				ExtraLabels:      map[string]string{"shared-label": "caller"},
				OutputNoPrint:    true,
				ReleaseName:      "release",
				ReleaseNamespace: "namespace",
				TempDirPath:      t.TempDir(),
			})
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)

			annotations := result.Resources[0].Unstruct.GetAnnotations()
			for key, value := range tt.wantAnnotations {
				assert.Equal(t, value, annotations[key], key)
			}

			assert.Len(t, lo.Intersect(lo.Keys(annotations), []string{"user-annotation", "project.werf.io/name"}), len(lo.Intersect(lo.Keys(tt.wantAnnotations), []string{"user-annotation", "project.werf.io/name"})))

			labels := result.Resources[0].Unstruct.GetLabels()
			for key, value := range tt.wantLabels {
				assert.Equal(t, value, labels[key], key)
			}

			_, hasUserLabel := labels["user-label"]
			assert.Equal(t, !tt.werfData, hasUserLabel)
		})
	}
}

func TestChartRender_RecordedRelease(t *testing.T) {
	tests := []struct {
		name          string
		metadata      string
		opts          ChartRenderOptions
		wantName      string
		wantNamespace string
	}{
		{
			name:          "recorded used when not given",
			metadata:      werfDataRecordedReleaseMetadata,
			wantName:      "progress-release",
			wantNamespace: "progress-ns",
		},
		{
			name:          "given ones win",
			metadata:      werfDataRecordedReleaseMetadata,
			opts:          ChartRenderOptions{ReleaseName: "given-release", ReleaseNamespace: "given-ns"},
			wantName:      "given-release",
			wantNamespace: "given-ns",
		},
		{
			name:          "recorded namespace fills in the missing one",
			metadata:      werfDataRecordedReleaseMetadata,
			opts:          ChartRenderOptions{ReleaseName: "given-release"},
			wantName:      "given-release",
			wantNamespace: "progress-ns",
		},
		{
			name:          "nothing recorded",
			metadata:      "apiVersion: v2\n",
			wantName:      common.StubReleaseName,
			wantNamespace: common.StubReleaseNamespace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			opts.Chart = newRecordedReleaseChart(t, tt.metadata)
			opts.OutputNoPrint = true
			opts.TempDirPath = t.TempDir()

			result, err := ChartRender(newChartRenderInstallContext(t), opts)
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)

			data, _, err := unstructured.NestedStringMap(result.Resources[0].Unstruct.Object, "data")
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"name": tt.wantName, "namespace": tt.wantNamespace}, data)
		})
	}
}

func TestChartRender_RecordedReleaseOfRemoteChart(t *testing.T) {
	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	t.Setenv("HELM_DATA_HOME", t.TempDir())

	host := newChartCopyTestRegistry(t)
	archivePath := filepath.Join(t.TempDir(), "app.tgz")
	require.NoError(t, os.WriteFile(archivePath, buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml":                        "apiVersion: v2\nname: app\nversion: 1.0.0\n",
		"templates/cm.yaml":                 recordedReleaseConfigMap,
		werfdata.DirName + "/metadata.yaml": werfDataRecordedReleaseMetadata,
	}), 0o644))

	_, err := ChartUpload(newChartPackTestContext(), archivePath, "oci://"+host, newChartUploadTestOptions(t))
	require.NoError(t, err)

	result, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
		Chart:                      "oci://" + host + "/app",
		ChartRepoConnectionOptions: common.ChartRepoConnectionOptions{ChartRepoInsecure: true},
		ChartVersion:               "1.0.0",
		OutputNoPrint:              true,
		RegistryCredentialsPath:    filepath.Join(t.TempDir(), "config.json"),
		TempDirPath:                t.TempDir(),
	})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)

	data, _, err := unstructured.NestedStringMap(result.Resources[0].Unstruct.Object, "data")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"name": "progress-release", "namespace": "progress-ns"}, data)
}

func TestChartRender_RemoteChartsRenderedFromPrivateSnapshots(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	t.Setenv("HELM_DATA_HOME", t.TempDir())

	hosts := []string{newChartCopyTestRegistry(t), newChartCopyTestRegistry(t)}
	for i, host := range hosts {
		archivePath := filepath.Join(t.TempDir(), "app.tgz")
		require.NoError(t, os.WriteFile(archivePath, buildChartCopyTestChart(t, map[string]string{
			"Chart.yaml":        "apiVersion: v2\nname: app\nversion: 1.0.0\n",
			"templates/cm.yaml": fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: registry\ndata:\n  registry: %q\n", strconv.Itoa(i)),
		}), 0o644))

		_, err := ChartUpload(newChartPackTestContext(), archivePath, "oci://"+host, newChartUploadTestOptions(t))
		require.NoError(t, err)
	}

	var wg sync.WaitGroup

	results := make([]*ChartRenderResultV2, len(hosts))

	errs := make([]error, len(hosts))
	for i, host := range hosts {
		wg.Go(func() {
			results[i], errs[i] = ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
				Chart:                   "oci://" + host + "/app:1.0.0",
				OutputNoPrint:           true,
				RegistryCredentialsPath: filepath.Join(t.TempDir(), "config.json"),
				TempDirPath:             t.TempDir(),
			})
		})
	}

	wg.Wait()

	for i := range hosts {
		require.NoError(t, errs[i])
		require.Len(t, results[i].Resources, 1)

		data, _, err := unstructured.NestedStringMap(results[i].Resources[0].Unstruct.Object, "data")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"registry": strconv.Itoa(i)}, data)
	}

	_, err := os.Stat(filepath.Join(cacheHome, "helm", "repository", "app-1.0.0.tgz"))
	assert.ErrorIs(t, err, fs.ErrNotExist, "the chart must not be downloaded into the shared repository cache")
}

func TestReleaseInstall_ChartAnnotations(t *testing.T) {
	for _, tt := range chartAnnotationsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhasesConfigMap(t, api)

			api.state.RWTransaction(func(s *progressPhasesAPIState) {
				s.allowApply = true
			})

			installOpts := newChartRenderInstallOptions(t, api, newChartAnnotationsTestChart(t, tt.werfData), nil)
			installOpts.ExtraAnnotations = chartAnnotationsTestCallerAnnotations
			installOpts.ExtraLabels = chartAnnotationsTestCallerLabels
			installOpts.ExtraRuntimeAnnotations = chartAnnotationsTestCallerRuntimeAnnotations
			installOpts.LegacyHelmCompatibleTracking = true
			installOpts.ReleaseInfoAnnotations = chartAnnotationsTestCallerRuntimeAnnotations

			ctx := newChartRenderInstallContext(t)

			_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", installOpts)
			require.NoError(t, err)

			var applied *corev1.ConfigMap
			api.state.RTransaction(func(s *progressPhasesAPIState) {
				for _, cm := range s.appliedConfigMaps {
					if cm.Name == "progress-resource" {
						applied = &cm
					}
				}
			})

			require.NotNil(t, applied)
			assertChartAnnotationsTestMetadata(t, applied.GetAnnotations(), lo.Assign(chartAnnotationsTestWantAnnotations, chartAnnotationsTestWantRuntimeAnnotations))
			assertChartAnnotationsTestMetadata(t, applied.GetLabels(), chartAnnotationsTestWantLabels)

			getResult, err := ReleaseGet(ctx, "progress-release", "progress-ns", ReleaseGetOptions{
				KubeConnectionOptions: api.kubeOpts,
				OutputNoPrint:         true,
				ReleaseStorageDriver:  "configmap",
				TempDirPath:           t.TempDir(),
			})
			require.NoError(t, err)
			assertChartAnnotationsTestMetadata(t, getResult.Release.Annotations, chartAnnotationsTestWantRuntimeAnnotations)
		})
	}
}

func TestReleaseInstall_RecordedRelease(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		wantErr  string
	}{
		{name: "recorded used when not given", metadata: werfDataRecordedReleaseMetadata},
		{
			name:     "nothing recorded",
			metadata: "apiVersion: v2\n",
			wantErr:  "release name and release namespace required: not given and not recorded in the chart's .werf-data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhasesConfigMap(t, api)

			api.state.RWTransaction(func(s *progressPhasesAPIState) {
				s.allowApply = true
			})

			installOpts := newChartRenderInstallOptions(t, api, newWerfDataChart(t, tt.metadata), nil)
			installOpts.LegacyHelmCompatibleTracking = true

			result, err := ReleaseInstall(newChartRenderInstallContext(t), "", "", installOpts)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "progress-release", result.Release.Name)
			assert.Equal(t, "progress-ns", result.Release.Namespace)
		})
	}
}

func TestReleaseInstall_ReleaseRequiredError(t *testing.T) {
	api := newProgressPhasesAPI(t)

	_, err := ReleaseInstall(newChartRenderInstallContext(t), "", "", newChartRenderInstallOptions(t, api, newWerfDataChart(t, "apiVersion: v2\n"), nil))

	var requiredErr *ReleaseRequiredError
	require.ErrorAs(t, err, &requiredErr)
	assert.Equal(t, &ReleaseRequiredError{MissingName: true, MissingNamespace: true}, requiredErr)
}

func TestReleasePlanInstall_ChartAnnotations(t *testing.T) {
	for _, tt := range chartAnnotationsTestCases {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhasesConfigMap(t, api)

			planArtifactPath := filepath.Join(t.TempDir(), "plan.gz")
			ctx := newChartRenderInstallContext(t)

			_, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
				KubeConnectionOptions: api.kubeOpts,
				Chart:                 newChartAnnotationsTestChart(t, tt.werfData),
				PlanArtifactPath:      planArtifactPath,
				ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
					ExtraAnnotations:          chartAnnotationsTestCallerAnnotations,
					ExtraLabels:               chartAnnotationsTestCallerLabels,
					ExtraRuntimeAnnotations:   chartAnnotationsTestCallerRuntimeAnnotations,
					ReleaseInfoAnnotations:    chartAnnotationsTestCallerRuntimeAnnotations,
					ReleaseStorageDriver:      "configmap",
					ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
				},
				TempDirPath: t.TempDir(),
			})
			require.NoError(t, err)

			planArtifact, err := ReadPlanArtifact(ctx, planArtifactPath, "", "")
			require.NoError(t, err)

			options := planArtifact.Data.Options
			assert.Equal(t, chartAnnotationsTestWantAnnotations, options.ExtraAnnotations)
			assert.Equal(t, chartAnnotationsTestWantLabels, options.ExtraLabels)
			assert.Equal(t, chartAnnotationsTestWantRuntimeAnnotations, options.ExtraRuntimeAnnotations)
			assert.Equal(t, chartAnnotationsTestWantRuntimeAnnotations, options.ReleaseInfoAnnotations)

			assert.Equal(t, chartAnnotationsTestCallerRuntimeAnnotations, planArtifact.Data.RollbackOptions.ExtraRuntimeAnnotations)
			assert.Equal(t, chartAnnotationsTestCallerRuntimeAnnotations, planArtifact.Data.RollbackOptions.ReleaseInfoAnnotations)
		})
	}
}

func TestReleasePlanInstall_LegacyBundleReleaseInfoAnnotations(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhasesConfigMap(t, api)

	chartPath := newRecordedReleaseChart(t, "apiVersion: v2\n")
	require.NoError(t, os.RemoveAll(filepath.Join(chartPath, werfdata.DirName)))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "extra_annotations.json"), []byte(`{"user-annotation": "chart", "project.werf.io/name": "app", "ci.werf.io/commit": "abc"}`), 0o600))

	planArtifactPath := filepath.Join(t.TempDir(), "plan.gz")
	ctx := newChartRenderInstallContext(t)

	_, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		Chart:                 chartPath,
		PlanArtifactPath:      planArtifactPath,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
			ReleaseInfoAnnotations:    map[string]string{"project.werf.io/name": "caller"},
			ReleaseStorageDriver:      "configmap",
			ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		},
		TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)

	planArtifact, err := ReadPlanArtifact(ctx, planArtifactPath, "", "")
	require.NoError(t, err)

	options := planArtifact.Data.Options
	assert.Equal(t, map[string]string{"user-annotation": "chart"}, options.ExtraAnnotations)
	assert.Equal(t, map[string]string{"project.werf.io/name": "app", "ci.werf.io/commit": "abc"}, options.ExtraRuntimeAnnotations)
	assert.Equal(t, map[string]string{"project.werf.io/name": "caller", "ci.werf.io/commit": "abc"}, options.ReleaseInfoAnnotations)
}

func TestReleasePlanInstall_RecordedRelease(t *testing.T) {
	tests := []struct {
		name        string
		metadata    string
		releaseName string
		wantErr     string
	}{
		{name: "recorded used when not given", metadata: werfDataRecordedReleaseMetadata},
		{name: "given name wins", metadata: werfDataRecordedReleaseMetadata, releaseName: "progress-release"},
		{
			name:     "nothing recorded",
			metadata: "apiVersion: v2\n",
			wantErr:  "release name and release namespace required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhasesConfigMap(t, api)

			planArtifactPath := filepath.Join(t.TempDir(), "plan.gz")
			ctx := newChartRenderInstallContext(t)

			_, err := ReleasePlanInstall(ctx, tt.releaseName, "", ReleasePlanInstallOptions{
				KubeConnectionOptions: api.kubeOpts,
				Chart:                 newWerfDataChart(t, tt.metadata),
				PlanArtifactPath:      planArtifactPath,
				ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
					ReleaseStorageDriver:      "configmap",
					ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
				},
				TempDirPath: t.TempDir(),
			})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			planArtifact, err := ReadPlanArtifact(ctx, planArtifactPath, "", "")
			require.NoError(t, err)
			assert.Equal(t, "progress-release", planArtifact.Release.Name)
			assert.Equal(t, "progress-ns", planArtifact.Release.Namespace)
		})
	}
}

func TestResolveRecordedRelease(t *testing.T) {
	tests := []struct {
		name             string
		metadata         string
		releaseName      string
		releaseNamespace string
		wantName         string
		wantNamespace    string
	}{
		{name: "recorded", metadata: werfDataRecordedReleaseMetadata, wantName: "progress-release", wantNamespace: "progress-ns"},
		{name: "given win", metadata: werfDataRecordedReleaseMetadata, releaseName: "given", releaseNamespace: "given-ns", wantName: "given", wantNamespace: "given-ns"},
		{name: "recorded fills in the empty one", metadata: werfDataRecordedReleaseMetadata, releaseName: "given", wantName: "given", wantNamespace: "progress-ns"},
		{name: "nothing recorded", metadata: "apiVersion: v2\n", releaseName: "given", wantName: "given"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, namespace, err := ResolveRecordedRelease(newChartPackTestContext(), newRecordedReleaseChart(t, tt.metadata), tt.releaseName, tt.releaseNamespace, ResolveRecordedReleaseOptions{})
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantNamespace, namespace)
		})
	}
}

func assertChartAnnotationsTestMetadata(t *testing.T, got, want map[string]string) {
	t.Helper()

	for key, value := range want {
		assert.Equal(t, value, got[key], key)
	}
}

// newChartAnnotationsTestChart returns a chart with the same annotations and labels recorded in
// .werf-data if werfData, else in the extra_annotations.json and extra_labels.json of a legacy bundle.
func newChartAnnotationsTestChart(t *testing.T, werfData bool) string {
	t.Helper()

	if werfData {
		return newWerfDataChart(t, `apiVersion: v2
extraAnnotations:
  user-annotation: chart
  shared-annotation: chart
extraLabels:
  user-label: chart
  shared-label: chart
extraRuntimeAnnotations:
  project.werf.io/name: chart
  ci.werf.io/commit: chart
  werf.io/release-channel: chart
releaseInfoAnnotations:
  project.werf.io/name: chart
  ci.werf.io/commit: chart
  werf.io/release-channel: chart
`)
	}

	chartPath := newWerfDataChart(t, "apiVersion: v2\n")
	require.NoError(t, os.RemoveAll(filepath.Join(chartPath, werfdata.DirName)))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "extra_annotations.json"), []byte(`{"user-annotation": "chart", "shared-annotation": "chart", "project.werf.io/name": "chart", "ci.werf.io/commit": "chart", "werf.io/release-channel": "chart"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "extra_labels.json"), []byte(`{"user-label": "chart", "shared-label": "chart"}`), 0o600))

	return chartPath
}

func newRecordedReleaseChart(t *testing.T, metadata string) string {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(path, "Chart.yaml"), []byte("apiVersion: v2\nname: app\nversion: 1.0.0\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(path, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "templates", "cm.yaml"), []byte(recordedReleaseConfigMap), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(path, werfdata.DirName), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, werfdata.DirName, werfdata.MetadataFileName), []byte(metadata), 0o600))

	return path
}
