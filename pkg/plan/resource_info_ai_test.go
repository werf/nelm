//go:build ai_tests

package plan

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

func TestAI_AdoptDeckhouseControllerFieldsGateEnvVarName(t *testing.T) {
	assert.Equal(t, "NELM_FEAT_ADOPT_DECKHOUSE_CONTROLLER_FIELDS", featgate.FeatGateAdoptDeckhouseControllerFields.EnvVarName())
}

func TestAI_ExclusiveOwnershipForOurManagerDeckhouseControllerGateDisabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Disable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	newManagedFields, _ := exclusiveOwnershipForOurManager([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry)

	assert.True(t, hasManager(newManagedFields, common.OldDeckhouseControllerManager))
}

func TestAI_ExclusiveOwnershipForOurManagerDeckhouseControllerGateEnabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Enable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	newManagedFields, changed := exclusiveOwnershipForOurManager([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry)

	assert.False(t, changed)
	assert.False(t, hasManager(newManagedFields, common.OldDeckhouseControllerManager))
}

func TestAI_FilterDelResourcesPresentInInstResources(t *testing.T) {
	for _, tt := range []struct {
		name         string
		instInfos    []*InstallableResourceInfo
		delInfos     []*DeletableResourceInfo
		expectedUIDs []types.UID
	}{
		{
			name:         "keeps every deletable when there are no installables",
			instInfos:    nil,
			delInfos:     []*DeletableResourceInfo{deletableInfoWithUID("a"), deletableInfoWithUID("b")},
			expectedUIDs: []types.UID{"a", "b"},
		},
		{
			name:         "returns nothing when there are no deletables",
			instInfos:    []*InstallableResourceInfo{installableInfoWithUID("a")},
			delInfos:     nil,
			expectedUIDs: []types.UID{},
		},
		{
			name:         "drops the deletable sharing a uid with an installable",
			instInfos:    []*InstallableResourceInfo{installableInfoWithUID("a"), installableInfoWithUID("b")},
			delInfos:     []*DeletableResourceInfo{deletableInfoWithUID("a"), deletableInfoWithUID("c")},
			expectedUIDs: []types.UID{"c"},
		},
		{
			name:         "drops every deletable when all of them share uids with installables",
			instInfos:    []*InstallableResourceInfo{installableInfoWithUID("a"), installableInfoWithUID("b")},
			delInfos:     []*DeletableResourceInfo{deletableInfoWithUID("b"), deletableInfoWithUID("a")},
			expectedUIDs: []types.UID{},
		},
		{
			name:         "ignores installables that were not found in the cluster",
			instInfos:    []*InstallableResourceInfo{{}, installableInfoWithUID("b")},
			delInfos:     []*DeletableResourceInfo{deletableInfoWithUID("a")},
			expectedUIDs: []types.UID{"a"},
		},
		{
			name:         "keeps deletables that were not found in the cluster",
			instInfos:    []*InstallableResourceInfo{installableInfoWithUID("a")},
			delInfos:     []*DeletableResourceInfo{{}, deletableInfoWithUID("a")},
			expectedUIDs: []types.UID{""},
		},
		{
			name:         "preserves the order of the kept deletables",
			instInfos:    []*InstallableResourceInfo{installableInfoWithUID("b")},
			delInfos:     []*DeletableResourceInfo{deletableInfoWithUID("a"), deletableInfoWithUID("b"), deletableInfoWithUID("c")},
			expectedUIDs: []types.UID{"a", "c"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterDelResourcesPresentInInstResources(tt.instInfos, tt.delInfos)

			assert.Equal(t, tt.expectedUIDs, lo.Map(filtered, func(info *DeletableResourceInfo, _ int) types.UID {
				if info.GetResult == nil {
					return ""
				}

				return info.GetResult.GetUID()
			}))
		})
	}
}

func TestAI_IterateInstallableResourceInfos(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		infos                []*InstallableResourceInfo
		expectedIterations   []int
		expectedInstallTypes []ResourceInstallType
	}{
		{
			name:                 "leaves a lone resource on the first iteration",
			infos:                []*InstallableResourceInfo{installableInfoNamed("a", ResourceInstallTypeApply)},
			expectedIterations:   []int{0},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply},
		},
		{
			name: "moves the second resource sharing an id to the next iteration",
			infos: []*InstallableResourceInfo{
				installableInfoNamed("a", ResourceInstallTypeApply),
				installableInfoNamed("a", ResourceInstallTypeApply),
			},
			expectedIterations:   []int{0, 1},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply, ResourceInstallTypeApply},
		},
		{
			name: "keeps the third resource sharing an id on the second iteration",
			infos: []*InstallableResourceInfo{
				installableInfoNamed("a", ResourceInstallTypeApply),
				installableInfoNamed("a", ResourceInstallTypeApply),
				installableInfoNamed("a", ResourceInstallTypeApply),
			},
			expectedIterations:   []int{0, 1, 1},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply, ResourceInstallTypeApply, ResourceInstallTypeApply},
		},
		{
			name: "iterates every id on its own",
			infos: []*InstallableResourceInfo{
				installableInfoNamed("a", ResourceInstallTypeApply),
				installableInfoNamed("b", ResourceInstallTypeApply),
				installableInfoNamed("a", ResourceInstallTypeApply),
				installableInfoNamed("b", ResourceInstallTypeApply),
				installableInfoNamed("a", ResourceInstallTypeApply),
			},
			expectedIterations: []int{0, 0, 1, 1, 1},
			expectedInstallTypes: []ResourceInstallType{
				ResourceInstallTypeApply,
				ResourceInstallTypeApply,
				ResourceInstallTypeApply,
				ResourceInstallTypeApply,
				ResourceInstallTypeApply,
			},
		},
		{
			name: "creates the next iteration of a resource deleted on successful install",
			infos: []*InstallableResourceInfo{
				installableInfoToDeleteOnSuccessfulInstall("a"),
				installableInfoNamed("a", ResourceInstallTypeApply),
			},
			expectedIterations:   []int{0, 1},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply, ResourceInstallTypeCreate},
		},
		{
			name: "keeps the install type of a resource with skip-create",
			infos: []*InstallableResourceInfo{
				installableInfoToDeleteOnSuccessfulInstall("a"),
				installableInfoNamed("a", ResourceInstallTypeApply, common.ResourcePolicySkipCreate),
			},
			expectedIterations:   []int{0, 1},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply, ResourceInstallTypeApply},
		},
		{
			name: "keeps a resource that must not be installed uninstalled",
			infos: []*InstallableResourceInfo{
				installableInfoToDeleteOnSuccessfulInstall("a"),
				installableInfoNamed("a", ResourceInstallTypeNone),
			},
			expectedIterations:   []int{0, 1},
			expectedInstallTypes: []ResourceInstallType{ResourceInstallTypeApply, ResourceInstallTypeNone},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			iterateInstallableResourceInfos(tt.infos)

			assert.Equal(t, tt.expectedIterations, lo.Map(tt.infos, func(info *InstallableResourceInfo, _ int) int {
				return info.Iteration
			}))

			assert.Equal(t, tt.expectedInstallTypes, lo.Map(tt.infos, func(info *InstallableResourceInfo, _ int) ResourceInstallType {
				return info.MustInstall
			}))
		})
	}
}

func TestAI_PoolRoutines(t *testing.T) {
	for _, tt := range []struct {
		name               string
		resourcesCount     int
		totalCount         int
		networkParallelism int
		expected           int
	}{
		{name: "all resources installable", resourcesCount: 100, totalCount: 100, networkParallelism: 30, expected: 30},
		{name: "mostly installable", resourcesCount: 100, totalCount: 105, networkParallelism: 30, expected: 28},
		{name: "even split", resourcesCount: 50, totalCount: 100, networkParallelism: 30, expected: 15},
		{name: "single resource of each kind", resourcesCount: 1, totalCount: 2, networkParallelism: 30, expected: 15},
		{name: "small share never drops below one", resourcesCount: 5, totalCount: 105, networkParallelism: 30, expected: 1},
		{name: "no resources", resourcesCount: 0, totalCount: 0, networkParallelism: 30, expected: 1},
		{name: "minimal parallelism", resourcesCount: 1, totalCount: 2, networkParallelism: 1, expected: 1},
		{name: "zero parallelism still yields a usable pool", resourcesCount: 1, totalCount: 2, networkParallelism: 0, expected: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, poolRoutines(tt.resourcesCount, tt.totalCount, tt.networkParallelism))
		})
	}
}

func TestAI_RemoveUndesirableManagersDeckhouseControllerGateDisabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Disable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	_, newOursEntry, changed := removeUndesirableManagers([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry, false)

	assert.False(t, changed)
	assert.Equal(t, `{"f:spec":{"f:foo":{}}}`, string(newOursEntry.FieldsV1.Raw))
}

func TestAI_RemoveUndesirableManagersDeckhouseControllerGateEnabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Enable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	_, newOursEntry, changed := removeUndesirableManagers([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry, false)

	assert.True(t, changed)
	assert.Contains(t, string(newOursEntry.FieldsV1.Raw), "f:bar")
}

func hasManager(managedFields []v1.ManagedFieldsEntry, manager string) bool {
	for _, managedField := range managedFields {
		if managedField.Manager == manager {
			return true
		}
	}

	return false
}

func testAIManagedFieldsEntry(manager string, operation v1.ManagedFieldsOperationType, rawFields string) v1.ManagedFieldsEntry {
	return v1.ManagedFieldsEntry{
		Manager:    manager,
		Operation:  operation,
		APIVersion: "apps/v1",
		FieldsType: "FieldsV1",
		FieldsV1:   &v1.FieldsV1{Raw: []byte(rawFields)},
	}
}
