package plan_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/dyntracker/statestore"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/plan"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

const (
	releaseTriggerTestNamespace = "test-namespace"
	releaseTriggerTestRelease   = "test-release"
)

var nonTriggeringAnnotations = map[string]string{"werf.io/triggers-release": "false"}

func TestPlanTriggersRelease(t *testing.T) {
	tests := []struct {
		name         string
		instInfos    func() []*plan.InstallableResourceInfo
		delInfos     func() []*plan.DeletableResourceInfo
		wantTriggers bool
	}{
		{
			name: "nothing to do",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeNone, false)}
			},
		},
		{
			name: "triggering resource recreated",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeRecreate, true)}
			},
			wantTriggers: true,
		},
		{
			name: "only non-triggering resource recreated",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{
					releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeNone, false),
					releaseTriggerTestInfo("job", nonTriggeringAnnotations, plan.ResourceInstallTypeRecreate, true),
				}
			},
		},
		{
			name: "only non-triggering resource created",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("job", nonTriggeringAnnotations, plan.ResourceInstallTypeCreate, true)}
			},
		},
		{
			name: "non-triggering resource recreated and triggering resource updated",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{
					releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeUpdate, true),
					releaseTriggerTestInfo("job", nonTriggeringAnnotations, plan.ResourceInstallTypeRecreate, true),
				}
			},
			wantTriggers: true,
		},
		{
			name: "non-triggering resource recreated with present dependency on unchanged resource",
			instInfos: func() []*plan.InstallableResourceInfo {
				job := releaseTriggerTestInfo("job", nonTriggeringAnnotations, plan.ResourceInstallTypeRecreate, true)
				job.LocalResource.ManualDependencies = []*resource.Dependency{releaseTriggerTestDependency("app", common.ResourceStatePresent)}

				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeNone, false), job}
			},
		},
		{
			name: "non-triggering resource recreated with ready dependency on unchanged resource",
			instInfos: func() []*plan.InstallableResourceInfo {
				job := releaseTriggerTestInfo("job", nonTriggeringAnnotations, plan.ResourceInstallTypeRecreate, true)
				job.LocalResource.ManualDependencies = []*resource.Dependency{releaseTriggerTestDependency("app", common.ResourceStateReady)}

				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeNone, true), job}
			},
		},
		{
			name: "only tracking of unchanged triggering resource",
			instInfos: func() []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{releaseTriggerTestInfo("app", nil, plan.ResourceInstallTypeNone, true)}
			},
			wantTriggers: true,
		},
		{
			name: "only non-triggering resource deleted",
			delInfos: func() []*plan.DeletableResourceInfo {
				return []*plan.DeletableResourceInfo{releaseTriggerTestDelInfo("job", nonTriggeringAnnotations)}
			},
		},
		{
			name: "triggering resource deleted",
			delInfos: func() []*plan.DeletableResourceInfo {
				return []*plan.DeletableResourceInfo{releaseTriggerTestDelInfo("app", nil)}
			},
			wantTriggers: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				instInfos []*plan.InstallableResourceInfo
				delInfos  []*plan.DeletableResourceInfo
			)
			if tt.instInfos != nil {
				instInfos = tt.instInfos()
			}

			if tt.delInfos != nil {
				delInfos = tt.delInfos()
			}

			p, err := plan.BuildPlan(context.Background(), instInfos, delInfos, nil, releaseTriggerTestNamespace, plan.BuildPlanOptions{})
			require.NoError(t, err)

			planJSON, err := json.Marshal(p)
			require.NoError(t, err)

			restoredPlan := plan.NewPlan()
			require.NoError(t, json.Unmarshal(planJSON, restoredPlan))

			for _, pln := range []*plan.Plan{p, restoredPlan} {
				assert.Equal(t, tt.wantTriggers, plan.PlanTriggersRelease(pln))
			}
		})
	}
}

func releaseTriggerTestDelInfo(name string, annotations map[string]string) *plan.DeletableResourceInfo {
	resSpec := releaseTriggerTestResourceSpec(name, annotations)

	return &plan.DeletableResourceInfo{
		ResourceMeta:     resSpec.ResourceMeta,
		GetResult:        resSpec.Unstruct,
		LocalResource:    &resource.DeletableResource{ResourceMeta: resSpec.ResourceMeta, Ownership: common.OwnershipRelease},
		MustDelete:       true,
		MustTrackAbsence: true,
		Stage:            common.StagePrePreUninstall,
	}
}

func releaseTriggerTestInfo(name string, annotations map[string]string, installType plan.ResourceInstallType, trackReadiness bool) *plan.InstallableResourceInfo {
	resSpec := releaseTriggerTestResourceSpec(name, annotations)

	info := &plan.InstallableResourceInfo{
		ResourceMeta: resSpec.ResourceMeta,
		LocalResource: &resource.InstallableResource{
			ResourceSpec:         resSpec,
			Ownership:            common.OwnershipRelease,
			FailMode:             statestore.FailWholeDeployProcessImmediately,
			TrackTerminationMode: statestore.WaitUntilResourceReady,
		},
		MustInstall:        installType,
		MustTrackReadiness: trackReadiness,
		FailMode:           statestore.FailWholeDeployProcessImmediately,
		Stage:              common.StageInstall,
	}

	if installType != plan.ResourceInstallTypeCreate {
		info.GetResult = resSpec.Unstruct
	}

	return info
}

func releaseTriggerTestDependency(name string, state common.ResourceState) *resource.Dependency {
	return &resource.Dependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:  []string{name},
			Groups: []string{""},
			Kinds:  []string{"ConfigMap"},
		},
		ResourceState: state,
	}
}

func releaseTriggerTestResourceSpec(name string, annotations map[string]string) *spec.ResourceSpec {
	unstruct := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": releaseTriggerTestNamespace,
		},
	}}
	unstruct.SetAnnotations(lo.Assign(map[string]string{
		"meta.helm.sh/release-name":      releaseTriggerTestRelease,
		"meta.helm.sh/release-namespace": releaseTriggerTestNamespace,
	}, annotations))

	return spec.NewResourceSpec(unstruct, releaseTriggerTestNamespace, spec.ResourceSpecOptions{})
}
