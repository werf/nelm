package action

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestApplyReleaseInstallOptionsDefaults_RejectsChartRenderResultWithPlanArtifact(t *testing.T) {
	opts := ReleaseInstallOptions{
		LegacyChartRenderResult: &ChartRenderResultV2{installable: &chartRenderInstallable{}},
		LegacyPlanArtifact:      &PlanArtifact{},
		TempDirPath:             t.TempDir(),
	}

	_, err := applyReleaseInstallOptionsDefaults(opts, t.TempDir(), t.TempDir())
	require.ErrorContains(t, err, "not supported together with plan artifact")
}

func TestApplyReleaseInstallOptionsDefaults_RejectsLocalChartRenderResult(t *testing.T) {
	opts := ReleaseInstallOptions{
		LegacyChartRenderResult: &ChartRenderResultV2{},
		TempDirPath:             t.TempDir(),
	}

	_, err := applyReleaseInstallOptionsDefaults(opts, t.TempDir(), t.TempDir())
	require.ErrorContains(t, err, "chart render result can't be installed")
}

func TestReleaseInstallChartRenderResultSkipsRenderingPhase(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)
	ctx := newChartRenderInstallContext(t)

	renderResult, err := ChartRender(ctx, newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release"))
	require.NoError(t, err)

	ch := make(chan progrep.ProgressReport, 4096)
	installOpts := newChartRenderInstallOptions(t, api, chartPath, renderResult)
	installOpts.LegacyProgressReportCh = ch
	installOpts.LegacyProgressReportPhases = true

	_, err = ReleaseInstall(ctx, "progress-release", "progress-ns", installOpts)
	require.NoError(t, err)

	reports := progressPhaseReports(t, ch)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting}, progressPhaseSequence(reports))
	assert.Nil(t, reports[len(reports)-1].RenderDetails)
}

func TestReleaseInstallChartRenderResultWithLegacyHelmCompatibleTracking(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)
	trackingAnnotations := map[string]string{
		common.AnnotationKeyHumanFailMode:             "IgnoreAndContinueDeployProcess",
		common.AnnotationKeyHumanTrackTerminationMode: "NonBlocking",
	}

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		cm := s.configMaps["progress-resource"]
		maps.Copy(cm.Annotations, trackingAnnotations)
		s.configMaps["progress-resource"] = cm
	})

	ctx := newChartRenderInstallContext(t)

	renderResult, err := ChartRender(ctx, newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release"))
	require.NoError(t, err)

	installOpts := newChartRenderInstallOptions(t, api, chartPath, renderResult)
	installOpts.LegacyHelmCompatibleTracking = true

	_, err = ReleaseInstall(ctx, "progress-release", "progress-ns", installOpts)
	require.NoError(t, err)

	getResult, err := ReleaseGet(ctx, "progress-release", "progress-ns", ReleaseGetOptions{
		KubeConnectionOptions: api.kubeOpts,
		OutputNoPrint:         true,
		ReleaseStorageDriver:  "configmap",
		TempDirPath:           t.TempDir(),
	})
	require.NoError(t, err)
	require.Len(t, getResult.ResourceSpecs, 1)

	for key, value := range trackingAnnotations {
		assert.Equal(t, value, getResult.ResourceSpecs[0].Unstruct.GetAnnotations()[key])
	}

	for _, res := range renderResult.installable.ResourceSpecs {
		assert.NotContains(t, res.Unstruct.GetAnnotations(), common.AnnotationKeyHumanFailMode)
	}
}

func TestReleaseInstallInstallsChartRenderResult(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		cm := s.configMaps["progress-resource"]
		cm.Labels["render-label"] = "render"
		cm.Data["key"] = "value-patched"
		s.configMaps["progress-resource"] = cm
	})

	ctx := newChartRenderInstallContext(t)
	patches := spec.Patches{Render: []spec.Patch{{Match: spec.ResourceMatcher{Kinds: []string{"ConfigMap"}}, Patch: `.data.key += "-patched"`}}}

	renderOpts := newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release")
	renderOpts.ExtraLabels = map[string]string{"render-label": "render"}
	renderOpts.ExtraRuntimeLabels = map[string]string{"render-runtime-label": "render"}
	renderOpts.LegacyPatches = patches

	renderResult, err := ChartRender(ctx, renderOpts)
	require.NoError(t, err)

	renderedKinds := map[string]common.StoreAs{}
	for _, res := range renderResult.Resources {
		renderedKinds[res.GroupVersionKind.Kind] = res.StoreAs

		if res.GroupVersionKind.Kind == "ConfigMap" {
			assert.Equal(t, "render", res.Unstruct.GetLabels()["render-runtime-label"])
		}
	}

	assert.Equal(t, map[string]common.StoreAs{"ConfigMap": common.StoreAsRegular, "CustomResourceDefinition": common.StoreAsNone}, renderedKinds)

	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(`{{ fail "chart rendered again" }}`), 0o600))

	installOpts := newChartRenderInstallOptions(t, api, chartPath, renderResult)
	installOpts.LegacyPatches = patches

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", installOpts)
	require.NoError(t, err)
	assert.Equal(t, helmreleasestatus.StatusDeployed, result.Release.Status)

	getResult, err := ReleaseGet(ctx, "progress-release", "progress-ns", ReleaseGetOptions{
		KubeConnectionOptions: api.kubeOpts,
		OutputNoPrint:         true,
		ReleaseStorageDriver:  "configmap",
		TempDirPath:           t.TempDir(),
	})
	require.NoError(t, err)
	require.Len(t, getResult.ResourceSpecs, 1)

	labels := getResult.ResourceSpecs[0].Unstruct.GetLabels()
	assert.Equal(t, "render", labels["render-label"])
	assert.NotContains(t, labels, "install-label")
	assert.NotContains(t, labels, "render-runtime-label")

	data, found, err := unstructured.NestedStringMap(getResult.ResourceSpecs[0].Unstruct.Object, "data")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "value-patched", data["key"])
}

func TestReleaseInstallRejectsChartRenderResultOfOtherDeployType(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)
	ctx := newChartRenderInstallContext(t)

	addProgressPhaseRevision(t, api, 1, "")

	renderResult, err := ChartRender(ctx, newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release"))
	require.NoError(t, err)

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		cm := s.configMaps["sh.helm.release.v1.progress-release.v1"]
		cm.Labels["status"] = "failed"
		s.configMaps["sh.helm.release.v1.progress-release.v1"] = cm
	})

	_, err = ReleaseInstall(ctx, "progress-release", "progress-ns", newChartRenderInstallOptions(t, api, chartPath, renderResult))
	require.ErrorContains(t, err, `chart render result deploy type mismatch: expected "Upgrade", got "Install"`)
}

func TestReleaseInstallRejectsChartRenderResultOfOtherRelease(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)
	ctx := newChartRenderInstallContext(t)

	renderResult, err := ChartRender(ctx, newChartRenderInstallRenderOptions(t, api, chartPath, "other-release"))
	require.NoError(t, err)

	_, err = ReleaseInstall(ctx, "progress-release", "progress-ns", newChartRenderInstallOptions(t, api, chartPath, renderResult))
	require.ErrorContains(t, err, `chart render result release mismatch: expected "other-release" (namespace: "progress-ns"), got "progress-release" (namespace: "progress-ns")`)
}

func TestReleaseInstallRejectsChartRenderResultOfOtherRevision(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newChartRenderInstallChart(t, api)
	ctx := newChartRenderInstallContext(t)

	renderResult, err := ChartRender(ctx, newChartRenderInstallRenderOptions(t, api, chartPath, "progress-release"))
	require.NoError(t, err)

	addProgressPhaseRevision(t, api, 1, "")

	_, err = ReleaseInstall(ctx, "progress-release", "progress-ns", newChartRenderInstallOptions(t, api, chartPath, renderResult))
	require.ErrorContains(t, err, "chart render result release revision mismatch: expected 1, got 2")
}
