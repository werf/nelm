package plan

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/featgate"
	"github.com/werf/nelm/v2/pkg/kube/fake"
	"github.com/werf/nelm/v2/pkg/resource"
)

func TestAdoptDeckhouseControllerFieldsGateEnvVarName(t *testing.T) {
	assert.Equal(t, "NELM_FEAT_ADOPT_DECKHOUSE_CONTROLLER_FIELDS", featgate.FeatGateAdoptDeckhouseControllerFields.EnvVarName())
}

func TestBuildResourceInfosFailedInputProgress(t *testing.T) {
	for _, installable := range []bool{true, false} {
		t.Run(lo.Ternary(installable, "installable", "deletable"), func(t *testing.T) {
			cf, err := fake.NewClientFactory(context.Background())
			require.NoError(t, err)

			getErr := errors.New("analysis get error")
			cf.Dynamic().(*dynamicfake.FakeDynamicClient).PrependReactor("get", "configmaps", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, getErr
			})

			var (
				instResources []*resource.InstallableResource
				delResources  []*resource.DeletableResource
			)

			resSpec := makeResourceSpec("failing", "default", gvkConfigMap)
			if installable {
				instResources = []*resource.InstallableResource{{
					ResourceSpec:     resSpec,
					DeployConditions: map[common.On][]common.Stage{common.InstallOnInstall: {common.StageInstall}},
				}}
			} else {
				delResources = []*resource.DeletableResource{{ResourceMeta: resSpec.ResourceMeta}}
			}

			calls := make(chan context.Context, 1)
			instInfos, delInfos, err := BuildResourceInfos(context.Background(), common.DeployTypeInitial, "release", "default", instResources, delResources, false, cf, BuildResourceInfosOptions{
				OnResourceAnalyzed: func(ctx context.Context) { calls <- ctx },
			})
			require.ErrorIs(t, err, getErr)
			assert.Nil(t, instInfos)
			assert.Nil(t, delInfos)
			assert.Empty(t, calls)
		})
	}
}

func TestBuildResourceInfosMultiStageProgress(t *testing.T) {
	for _, skipCreate := range []bool{false, true} {
		t.Run(lo.Ternary(skipCreate, "policy skip", "create"), func(t *testing.T) {
			cf, err := fake.NewClientFactory(context.Background())
			require.NoError(t, err)

			res := &resource.InstallableResource{
				ResourceSpec: makeResourceSpec("staged", "default", gvkConfigMap),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StagePreInstall, common.StagePostInstall},
				},
			}
			res.Unstruct = &unstructured.Unstructured{}
			res.Unstruct.SetGroupVersionKind(gvkConfigMap)

			if skipCreate {
				res.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipCreate}
			}

			calls := make(chan context.Context, 2)
			instInfos, delInfos, err := BuildResourceInfos(context.Background(), common.DeployTypeInitial, "release", "default", []*resource.InstallableResource{res}, nil, false, cf, BuildResourceInfosOptions{
				OnResourceAnalyzed: func(ctx context.Context) { calls <- ctx },
			})
			require.NoError(t, err)
			require.Len(t, instInfos, 2)
			assert.Empty(t, delInfos)
			assert.Len(t, calls, 1)
			assert.Equal(t, common.StagePreInstall, instInfos[0].Stage)
			assert.Equal(t, common.StagePostInstall, instInfos[1].Stage)

			for _, info := range instInfos {
				assert.Equal(t, lo.Ternary(skipCreate, ResourceInstallTypeNone, ResourceInstallTypeCreate), info.MustInstall)
			}
		})
	}
}

func TestBuildResourceInfosNoInputsProgress(t *testing.T) {
	cf, err := fake.NewClientFactory(context.Background())
	require.NoError(t, err)

	calls := make(chan context.Context, 1)
	instInfos, delInfos, err := BuildResourceInfos(context.Background(), common.DeployTypeInitial, "release", "default", nil, nil, false, cf, BuildResourceInfosOptions{
		OnResourceAnalyzed: func(ctx context.Context) { calls <- ctx },
	})
	require.NoError(t, err)
	assert.Empty(t, instInfos)
	assert.Empty(t, delInfos)
	assert.Empty(t, calls)
}

func TestBuildResourceInfosSkippedInputsProgress(t *testing.T) {
	for _, withCallback := range []bool{false, true} {
		t.Run(lo.Ternary(withCallback, "callback", "nil callback"), func(t *testing.T) {
			cf, err := fake.NewClientFactory(context.Background())
			require.NoError(t, err)

			instResources := []*resource.InstallableResource{
				{ResourceSpec: makeResourceSpec("a", "default", gvkConfigMap)},
				{ResourceSpec: makeResourceSpec("b", "default", gvkConfigMap)},
				{ResourceSpec: makeResourceSpec("c", "default", gvkConfigMap)},
			}
			delRes := &resource.DeletableResource{
				ResourceMeta:     makeResourceMeta("duplicate", "default", gvkConfigMap),
				ResourcePolicies: []common.ResourcePolicy{common.ResourcePolicySkipDelete},
			}
			delResources := []*resource.DeletableResource{delRes, delRes, delRes}
			calls := make(chan context.Context, len(instResources)+len(delResources))

			opts := BuildResourceInfosOptions{NetworkParallelism: 10}
			if withCallback {
				opts.OnResourceAnalyzed = func(ctx context.Context) { calls <- ctx }
			}

			type contextKey struct{}

			key := contextKey{}
			ctx := context.WithValue(context.Background(), key, "progress")
			instInfos, delInfos, err := BuildResourceInfos(ctx, common.DeployTypeInitial, "release", "default", instResources, delResources, false, cf, opts)
			require.NoError(t, err)
			assert.Empty(t, instInfos)
			require.Len(t, delInfos, 1)
			assert.False(t, delInfos[0].MustDelete)
			assert.Len(t, calls, lo.Ternary(withCallback, 6, 0))
			close(calls)

			for workerCtx := range calls {
				assert.Equal(t, "progress", workerCtx.Value(key))
				assert.NotSame(t, ctx, workerCtx)
			}
		})
	}
}

func TestExclusiveOwnershipForOurManagerDeckhouseControllerGateDisabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Disable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	newManagedFields, _ := exclusiveOwnershipForOurManager([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry)

	assert.True(t, hasManager(newManagedFields, common.OldDeckhouseControllerManager))
}

func TestExclusiveOwnershipForOurManagerDeckhouseControllerGateEnabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Enable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	newManagedFields, changed := exclusiveOwnershipForOurManager([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry)

	assert.False(t, changed)
	assert.False(t, hasManager(newManagedFields, common.OldDeckhouseControllerManager))
}

func TestFilterDelResourcesPresentInInstResources(t *testing.T) {
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

func TestIterateInstallableResourceInfos(t *testing.T) {
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

func TestPoolRoutines(t *testing.T) {
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

func TestRemoveUndesirableManagersDeckhouseControllerGateDisabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Disable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	_, newOursEntry, changed := removeUndesirableManagers([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry, false)

	assert.False(t, changed)
	assert.JSONEq(t, `{"f:spec":{"f:foo":{}}}`, string(newOursEntry.FieldsV1.Raw))
}

func TestRemoveUndesirableManagersDeckhouseControllerGateEnabled(t *testing.T) {
	featgate.FeatGateAdoptDeckhouseControllerFields.Enable()
	t.Cleanup(featgate.FeatGateAdoptDeckhouseControllerFields.Disable)

	oursEntry := testAIManagedFieldsEntry(common.DefaultFieldManager, v1.ManagedFieldsOperationApply, `{"f:spec":{"f:foo":{}}}`)
	deckhouseEntry := testAIManagedFieldsEntry(common.OldDeckhouseControllerManager, v1.ManagedFieldsOperationUpdate, `{"f:spec":{"f:bar":{},"f:foo":{}}}`)

	_, newOursEntry, changed := removeUndesirableManagers([]v1.ManagedFieldsEntry{deckhouseEntry}, oursEntry, false)

	assert.True(t, changed)
	assert.Contains(t, string(newOursEntry.FieldsV1.Raw), "f:bar")
}

func TestResourceInstallTypeOnImmutableStatefulSetError(t *testing.T) {
	statefulSetErr := apierrors.NewInvalid(schema.GroupKind{Group: "apps", Kind: "StatefulSet"}, "db", field.ErrorList{
		field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'ordinals', 'template', 'updateStrategy', 'revisionHistoryLimit', 'persistentVolumeClaimRetentionPolicy' and 'minReadySeconds' are forbidden"),
	})
	liveObj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "StatefulSet"}}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}

	recreateOnImmutable := &resource.InstallableResource{ResourceSpec: makeResourceSpec("db", "default", gvk), RecreateOnImmutable: true}
	installType, skipped, err := resourceInstallType(context.Background(), recreateOnImmutable, liveObj, nil, statefulSetErr, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, ResourceInstallTypeRecreate, installType)
	assert.False(t, skipped)

	noPolicy := &resource.InstallableResource{ResourceSpec: makeResourceSpec("db", "default", gvk)}
	_, _, err = resourceInstallType(context.Background(), noPolicy, liveObj, nil, statefulSetErr, nil, nil, nil, nil)
	require.ErrorContains(t, err, "immutable fields change in resource")
	require.ErrorIs(t, err, statefulSetErr)

	installType, skipped, err = resourceInstallType(context.Background(), noPolicy, liveObj, nil, statefulSetErr, nil, nil, []common.ResourcePolicy{common.ResourcePolicySkipRecreate}, nil)
	require.NoError(t, err)
	assert.Equal(t, ResourceInstallTypeNone, installType)
	assert.True(t, skipped)
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
