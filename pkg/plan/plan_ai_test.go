//go:build ai_tests

package plan

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/common"
)

func TestAI_SquashFinalTrackingOperationsMatchesReference(t *testing.T) {
	for _, tt := range []struct {
		name       string
		opsCount   int
		edgeChance float64
	}{
		{name: "sparse graph", opsCount: 14, edgeChance: 0.12},
		{name: "dense graph", opsCount: 14, edgeChance: 0.55},
		{name: "sparse large graph", opsCount: 70, edgeChance: 0.04},
		{name: "dense large graph", opsCount: 70, edgeChance: 0.25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for seed := int64(1); seed <= 40; seed++ {
				categories, deps := randomTrackingGraph(rand.New(rand.NewSource(seed)), tt.opsCount, tt.edgeChance)

				actual := buildTestPlan(trackingTestOperations(categories), deps)
				squashFinalTrackingOperations(actual)

				expected := buildTestPlan(trackingTestOperations(categories), deps)
				squashFinalTrackingOperationsReference(expected)

				require.Equal(t, lo.Must(expected.Graph.AdjacencyMap()), lo.Must(actual.Graph.AdjacencyMap()), "seed %d", seed)
			}
		})
	}
}

func TestAI_StageOperationID(t *testing.T) {
	for _, stage := range common.StagesOrdered {
		for _, suffix := range []string{common.StageStartSuffix, common.StageEndSuffix} {
			op := &Operation{
				Type:     OperationTypeNoop,
				Version:  OperationVersionNoop,
				Category: OperationCategoryMeta,
				Config: &OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, stage, suffix),
				},
			}

			assert.Equal(t, op.ID(), stageOperationID(stage, suffix))
		}
	}
}
