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
				categories, deps := randomTrackingGraph(rand.New(rand.NewSource(seed)), tt.opsCount, tt.edgeChance) //nolint:gosec

				actual := buildTestPlan(trackingTestOperations(categories), deps)
				squashFinalTrackingOperations(actual)

				expected := buildTestPlan(trackingTestOperations(categories), deps)
				squashFinalTrackingOperationsReference(expected)

				require.Equal(t, lo.Must(expected.Graph.AdjacencyMap()), lo.Must(actual.Graph.AdjacencyMap()), "seed %d", seed)
			}
		})
	}
}

func TestAI_SquashFinalTrackingOperationsSemantics(t *testing.T) {
	for _, tt := range []struct {
		name          string
		categories    []OperationCategory
		deps          map[int][]int
		expectedOps   []int
		expectedEdges [][2]int
	}{
		{
			name:          "squashes tracking with no resource operation below it",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack},
			deps:          map[int][]int{1: {0}},
			expectedOps:   []int{0},
			expectedEdges: nil,
		},
		{
			name:          "keeps tracking that a resource operation depends on",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryResource},
			deps:          map[int][]int{1: {0}, 2: {1}},
			expectedOps:   []int{0, 1, 2},
			expectedEdges: [][2]int{{0, 1}, {1, 2}},
		},
		{
			name:          "keeps tracking that reaches a resource operation through a meta operation",
			categories:    []OperationCategory{OperationCategoryTrack, OperationCategoryMeta, OperationCategoryResource},
			deps:          map[int][]int{1: {0}, 2: {1}},
			expectedOps:   []int{0, 1, 2},
			expectedEdges: [][2]int{{0, 1}, {1, 2}},
		},
		{
			name:          "reconnects predecessors to successors of the squashed tracking",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryMeta},
			deps:          map[int][]int{1: {0}, 2: {1}},
			expectedOps:   []int{0, 2},
			expectedEdges: [][2]int{{0, 2}},
		},
		{
			name:          "squashes tracking followed only by a release operation",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryRelease},
			deps:          map[int][]int{1: {0}, 2: {1}},
			expectedOps:   []int{0, 2},
			expectedEdges: [][2]int{{0, 2}},
		},
		{
			name:          "squashes a whole chain of trailing tracking operations",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryTrack},
			deps:          map[int][]int{1: {0}, 2: {1}},
			expectedOps:   []int{0},
			expectedEdges: nil,
		},
		{
			name:          "squashes only the trailing tracking of a branching graph",
			categories:    []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryTrack, OperationCategoryResource},
			deps:          map[int][]int{1: {0}, 2: {0}, 3: {2}},
			expectedOps:   []int{0, 2, 3},
			expectedEdges: [][2]int{{0, 2}, {2, 3}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ops := trackingTestOperations(tt.categories)

			p := buildTestPlan(ops, tt.deps)
			squashFinalTrackingOperations(p)

			adjMap := lo.Must(p.Graph.AdjacencyMap())

			assert.ElementsMatch(t, lo.Map(tt.expectedOps, func(i, _ int) string {
				return ops[i].ID()
			}), lo.Keys(adjMap))

			var actualEdges []string
			for fromID, adjacencies := range adjMap {
				for toID := range adjacencies {
					actualEdges = append(actualEdges, fromID+" -> "+toID)
				}
			}

			assert.ElementsMatch(t, lo.Map(tt.expectedEdges, func(edge [2]int, _ int) string {
				return ops[edge[0]].ID() + " -> " + ops[edge[1]].ID()
			}), actualEdges)
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
