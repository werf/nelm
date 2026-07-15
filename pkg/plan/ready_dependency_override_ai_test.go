//go:build ai_tests

package plan_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/kube/fake"
	"github.com/werf/nelm/pkg/plan"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/spec"
)

const (
	readyDepReleaseName      = "test-release"
	readyDepReleaseNamespace = "test-namespace"
)

func TestAI_ReadyDependencyCrossStageDoesNotForceTracking(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)

	dependent := readyDepInstallableResource(
		readyDepConfigMapSpec("dependent", readyDepReleaseNamespace, nil),
		multitrack.WaitUntilResourceReady,
		multitrack.FailWholeDeployProcessImmediately,
	)
	dependent.DeployConditions = map[common.On][]common.Stage{
		common.InstallOnInstall:  {common.StagePostInstall},
		common.InstallOnUpgrade:  {common.StagePostInstall},
		common.InstallOnRollback: {common.StagePostInstall},
	}
	dependent.ManualInternalDependencies = []*resource.InternalDependency{
		{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:  []string{"target"},
				Groups: []string{""},
				Kinds:  []string{"ConfigMap"},
			},
			ResourceState: common.ResourceStateReady,
		},
	}

	infos := buildReadyDepInfos(t, target, dependent, nil)

	targetInfo := findInfo(t, infos, "target")
	require.False(t, targetInfo.MustTrackReadiness,
		"a cross-stage state=ready dep must not force tracking, since no ordering edge can form")
	require.Equal(t, multitrack.IgnoreAndContinueDeployProcess, targetInfo.FailMode,
		"fail mode must remain the resource's own value when not forced")
}

func TestAI_ReadyDependencyDoesNotForceCRDTarget(t *testing.T) {
	crdInfo := &plan.InstallableResourceInfo{
		ResourceMeta: &spec.ResourceMeta{
			Name:             "widgets.example.com",
			GroupVersionKind: schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		},
		LocalResource: &resource.InstallableResource{},
		MustInstall:   plan.ResourceInstallTypeCreate,
		Stage:         common.StagePrePreInstall,
	}

	// A Namespace also deploys in pre-pre-install, so it shares a stage with a CRD and can carry
	// a ready dependency targeting one; the CRD must still be excluded from forced tracking.
	dependentInfo := &plan.InstallableResourceInfo{
		ResourceMeta: &spec.ResourceMeta{
			Name:             "my-namespace",
			GroupVersionKind: schema.GroupVersionKind{Version: "v1", Kind: "Namespace"},
		},
		LocalResource: &resource.InstallableResource{
			ManualInternalDependencies: []*resource.InternalDependency{
				{
					ResourceMatcher: &spec.ResourceMatcher{
						Names:  []string{"widgets.example.com"},
						Groups: []string{"apiextensions.k8s.io"},
						Kinds:  []string{"CustomResourceDefinition"},
					},
					ResourceState: common.ResourceStateReady,
				},
			},
		},
		Stage: common.StagePrePreInstall,
	}

	plan.ForceReadinessTrackingForReadyDependencyTargets([]*plan.InstallableResourceInfo{crdInfo, dependentInfo})

	require.False(t, crdInfo.MustTrackReadiness,
		"a CRD target must never be forced to track even when a same-stage dependent selects it")
	require.NotEqual(t, multitrack.FailWholeDeployProcessImmediately, crdInfo.FailMode)
}

func TestAI_ReadyDependencyDoesNotForceSkipCreateAbsentTarget(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)
	target.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipCreate}
	dependent := readyDependentResource("target", "")

	infos := buildReadyDepInfos(t, target, dependent, nil)

	targetInfo := findInfo(t, infos, "target")
	require.Equal(t, plan.ResourceInstallTypeNone, targetInfo.MustInstall)
	require.Nil(t, targetInfo.GetResult, "target is absent")
	require.False(t, targetInfo.MustTrackReadiness,
		"an absent skip-create target that will never be created must not be force-tracked")
	require.Equal(t, multitrack.IgnoreAndContinueDeployProcess, targetInfo.FailMode,
		"fail mode must remain the resource's own value when not forced")
}

func TestAI_ReadyDependencyDoesNotForceUnmatchedTarget(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)
	dependent := readyDependentResource("some-other-name", "")

	infos := buildReadyDepInfos(t, target, dependent, nil)

	targetInfo := findInfo(t, infos, "target")
	require.False(t, targetInfo.MustTrackReadiness, "NonBlocking target not selected by any dep stays untracked")
	require.Equal(t, multitrack.IgnoreAndContinueDeployProcess, targetInfo.FailMode,
		"fail mode must remain the resource's own value when not forced")
}

func TestAI_ReadyDependencyForcesTrackingOnChartAuthoredNonBlockingTarget(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, map[string]string{
			"werf.io/track-termination-mode": "NonBlocking",
			"werf.io/fail-mode":              "IgnoreAndContinueDeployProcess",
		}),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)
	dependent := readyDependentResource("target", "")

	infos := buildReadyDepInfos(t, target, dependent, nil)

	targetInfo := findInfo(t, infos, "target")
	require.True(t, targetInfo.MustTrackReadiness)
	require.Equal(t, multitrack.FailWholeDeployProcessImmediately, targetInfo.FailMode)
}

func TestAI_ReadyDependencyForcesTrackingOnLegacyPatchedTarget(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)
	dependent := readyDependentResource("target", "")

	infos := buildReadyDepInfos(t, target, dependent, nil)

	targetInfo := findInfo(t, infos, "target")
	require.True(t, targetInfo.MustTrackReadiness, "NonBlocking target selected by state=ready dep must be tracked")
	require.Equal(t, multitrack.FailWholeDeployProcessImmediately, targetInfo.FailMode)
}

func TestAI_ReadyDependencyForcesTrackingOnUnchangedTarget(t *testing.T) {
	targetSpec := readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil)
	target := readyDepInstallableResource(targetSpec, multitrack.NonBlocking, multitrack.IgnoreAndContinueDeployProcess)
	dependent := readyDependentResource("target", "")

	infos := buildReadyDepInfos(t, target, dependent, func(cf *fake.ClientFactory) {
		_, err := cf.KubeClient().Create(context.Background(), targetSpec, kube.KubeClientCreateOptions{
			DefaultNamespace: readyDepReleaseNamespace,
		})
		require.NoError(t, err)
	})

	targetInfo := findInfo(t, infos, "target")
	require.Equal(t, plan.ResourceInstallTypeNone, targetInfo.MustInstall, "target must be an unchanged no-op")
	require.True(t, targetInfo.MustTrackReadiness, "unchanged target still forced to track when selected by state=ready dep")
	require.Equal(t, multitrack.FailWholeDeployProcessImmediately, targetInfo.FailMode)
}

func TestAI_ReadyDependencyProducesEdgeAndRetainsTrackingWithNoFinalTracking(t *testing.T) {
	target := readyDepInstallableResource(
		readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil),
		multitrack.NonBlocking,
		multitrack.IgnoreAndContinueDeployProcess,
	)
	dependent := readyDependentResource("target", "")

	infos := buildReadyDepInfos(t, target, dependent, nil)

	p, err := plan.BuildPlan(infos, nil, nil, plan.BuildPlanOptions{NoFinalTracking: true})
	require.NoError(t, err)

	trackOp := findTrackReadinessOp(t, p, "target")
	createDependentOp := findCreateOp(t, p, "dependent")
	createTargetOp := findCreateOp(t, p, "target")

	require.True(t, planHasEdge(t, p, createTargetOp.ID(), trackOp.ID()),
		"target create must precede its track-readiness op")
	require.True(t, planHasEdge(t, p, trackOp.ID(), createDependentOp.ID()),
		"target track-readiness must precede dependent create (ready-dependency edge)")

	cfg := trackOp.Config.(*plan.OperationConfigTrackReadiness)
	require.Equal(t, multitrack.FailWholeDeployProcessImmediately, cfg.FailMode,
		"forced readiness op must fail the whole deploy despite IgnoreAndContinue annotation")
}

func TestAI_ReadyDependencyReleaseNamespaceSelectorProducesEdge(t *testing.T) {
	cf, err := fake.NewClientFactory(context.Background())
	require.NoError(t, err)

	targetSpec := readyDepConfigMapSpec("target", readyDepReleaseNamespace, nil)
	target, err := resource.NewInstallableResource(targetSpec, nil, readyDepReleaseNamespace, cf, resource.InstallableResourceOptions{})
	require.NoError(t, err)

	dependentSpec := readyDepConfigMapSpec("dependent", readyDepReleaseNamespace, map[string]string{
		"werf.io/deploy-dependency-target": "state=ready,version=v1,kind=ConfigMap,name=target,namespace=" + readyDepReleaseNamespace,
	})
	dependent, err := resource.NewInstallableResource(dependentSpec, nil, readyDepReleaseNamespace, cf, resource.InstallableResourceOptions{})
	require.NoError(t, err)

	instInfos, _, err := plan.BuildResourceInfos(
		context.Background(),
		common.DeployTypeInitial,
		readyDepReleaseName,
		readyDepReleaseNamespace,
		[]*resource.InstallableResource{target, dependent},
		nil,
		false,
		cf,
		plan.BuildResourceInfosOptions{NetworkParallelism: 10},
	)
	require.NoError(t, err)

	p, err := plan.BuildPlan(instInfos, nil, nil, plan.BuildPlanOptions{NoFinalTracking: true})
	require.NoError(t, err)

	trackOp := findTrackReadinessOp(t, p, "target")
	createDependentOp := findCreateOp(t, p, "dependent")

	require.True(t, planHasEdge(t, p, trackOp.ID(), createDependentOp.ID()),
		"a namespace=<release-ns> selector must normalize to the target's empty namespace and produce the ready-dependency edge")
}

func readyDependentResource(targetName, targetNamespace string) *resource.InstallableResource {
	res := readyDepInstallableResource(
		readyDepConfigMapSpec("dependent", readyDepReleaseNamespace, nil),
		multitrack.WaitUntilResourceReady,
		multitrack.FailWholeDeployProcessImmediately,
	)
	res.ManualInternalDependencies = []*resource.InternalDependency{
		{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:      []string{targetName},
				Namespaces: []string{targetNamespace},
				Groups:     []string{""},
				Kinds:      []string{"ConfigMap"},
			},
			ResourceState: common.ResourceStateReady,
		},
	}

	return res
}

func buildReadyDepInfos(t *testing.T, target, dependent *resource.InstallableResource, prepare func(cf *fake.ClientFactory)) []*plan.InstallableResourceInfo {
	t.Helper()

	cf, err := fake.NewClientFactory(context.Background())
	require.NoError(t, err)

	if prepare != nil {
		prepare(cf)
	}

	instInfos, _, err := plan.BuildResourceInfos(
		context.Background(),
		common.DeployTypeInitial,
		readyDepReleaseName,
		readyDepReleaseNamespace,
		[]*resource.InstallableResource{target, dependent},
		nil,
		false,
		cf,
		plan.BuildResourceInfosOptions{NetworkParallelism: 10},
	)
	require.NoError(t, err)

	return instInfos
}

func findCreateOp(t *testing.T, p *plan.Plan, name string) *plan.Operation {
	t.Helper()

	op, found := lo.Find(p.Operations(), func(op *plan.Operation) bool {
		cfg, ok := op.Config.(*plan.OperationConfigCreate)

		return ok && cfg.ResourceSpec.Name == name
	})
	require.Truef(t, found, "create op for %q not found", name)

	return op
}

func findInfo(t *testing.T, infos []*plan.InstallableResourceInfo, name string) *plan.InstallableResourceInfo {
	t.Helper()

	info, found := lo.Find(infos, func(i *plan.InstallableResourceInfo) bool {
		return i.Name == name
	})
	require.Truef(t, found, "info for %q not found", name)

	return info
}

func findTrackReadinessOp(t *testing.T, p *plan.Plan, name string) *plan.Operation {
	t.Helper()

	op, found := lo.Find(p.Operations(), func(op *plan.Operation) bool {
		cfg, ok := op.Config.(*plan.OperationConfigTrackReadiness)

		return ok && cfg.ResourceMeta.Name == name
	})
	require.Truef(t, found, "track-readiness op for %q not found", name)

	return op
}

func planHasEdge(t *testing.T, p *plan.Plan, fromID, toID string) bool {
	t.Helper()

	adjMap, err := p.Graph.AdjacencyMap()
	require.NoError(t, err)

	_, ok := adjMap[fromID][toID]

	return ok
}

func readyDepConfigMapSpec(name, namespace string, annotations map[string]string) *spec.ResourceSpec {
	meta := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
		"annotations": map[string]interface{}{
			"meta.helm.sh/release-name":      readyDepReleaseName,
			"meta.helm.sh/release-namespace": readyDepReleaseNamespace,
		},
		"labels": map[string]interface{}{
			"app.kubernetes.io/managed-by": "Helm",
		},
	}
	anns := meta["annotations"].(map[string]interface{})
	for k, v := range annotations {
		anns[k] = v
	}

	resSpec := spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   meta,
			"data":       map[string]interface{}{"key": "value"},
		},
	}, namespace, spec.ResourceSpecOptions{})
	resSpec.Unstruct.SetNamespace(namespace)

	return resSpec
}

func readyDepInstallableResource(resSpec *spec.ResourceSpec, trackTermination multitrack.TrackTerminationMode, failMode multitrack.FailMode) *resource.InstallableResource {
	return &resource.InstallableResource{
		ResourceSpec:                    resSpec,
		Ownership:                       common.OwnershipRelease,
		FailMode:                        failMode,
		NoActivityTimeout:               4 * time.Minute,
		ShowLogsOnlyForNumberOfReplicas: 1,
		TrackTerminationMode:            trackTermination,
		Weight:                          lo.ToPtr(0),
		DeployConditions: map[common.On][]common.Stage{
			common.InstallOnInstall:  {common.StageInstall},
			common.InstallOnUpgrade:  {common.StageInstall},
			common.InstallOnRollback: {common.StageInstall},
		},
	}
}
