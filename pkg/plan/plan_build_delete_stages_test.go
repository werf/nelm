package plan_test

import (
	"context"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/plan"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestBuildPlanSharesExternalDeleteDependencyAcrossStages(t *testing.T) {
	const (
		releaseName      = "test-release"
		releaseNamespace = "test-namespace"
	)

	externalDep := &resource.Dependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{"external"},
			Namespaces: []string{releaseNamespace},
			Versions:   []string{"v1"},
			Kinds:      []string{"ConfigMap"},
		},
		ResourceState: common.ResourceStateAbsent,
		External:      true,
	}

	newDelInfo := func(name string, stage common.Stage) *plan.DeletableResourceInfo {
		delRes := defaultDeletableResource(releaseName, releaseNamespace)
		delRes.Name = name
		delRes.ManualDependencies = []*resource.Dependency{externalDep}

		delInfo := defaultDeletableResourceInfo(delRes, releaseName, releaseNamespace)
		delInfo.Stage = stage

		return delInfo
	}

	laterStageDelInfo := newDelInfo("aa-later-stage", common.StagePostUninstall)
	earlierStageDelInfo := newDelInfo("zz-earlier-stage", common.StagePreUninstall)

	p, err := plan.BuildPlan(context.Background(), nil, []*plan.DeletableResourceInfo{laterStageDelInfo, earlierStageDelInfo}, nil, releaseNamespace, plan.BuildPlanOptions{})
	require.NoError(t, err)

	externalMeta := resource.NewResourceMetaFromDependency(externalDep, releaseNamespace)
	trackOpID := plan.OperationID(plan.OperationTypeTrackAbsence, plan.OperationVersionTrackAbsence, plan.OperationIteration(0), externalMeta.ID())

	for _, delInfo := range []*plan.DeletableResourceInfo{earlierStageDelInfo, laterStageDelInfo} {
		deleteOpID := plan.OperationID(plan.OperationTypeDelete, plan.OperationVersionDelete, plan.OperationIteration(0), delInfo.ID())

		_, err := graph.ShortestPath(p.Graph, trackOpID, deleteOpID)
		require.NoError(t, err, "no path from %q to %q", trackOpID, deleteOpID)
	}
}
