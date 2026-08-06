//go:build ai_tests

package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
