package resource_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

const deleteDependencyAnnotation = "werf.io/delete-dependency-target"

var incompleteSavedDeleteDependencies = []struct {
	name         string
	value        string
	missingField string
}{
	{name: "missing name", value: "state=absent,group=apps,version=v1,kind=Deployment", missingField: "name"},
	{name: "missing kind", value: "state=absent,group=apps,version=v1,name=absent-target", missingField: "kind"},
	{name: "missing version", value: "state=absent,group=apps,kind=Deployment,name=absent-target", missingField: "version"},
}

func TestBuildResourcesDeleteDependencyRequiresState(t *testing.T) {
	tests := []struct {
		name          string
		deployType    common.DeployType
		previous      bool
		expectedStage string
	}{
		{name: "upgrade saved annotation", deployType: common.DeployTypeUpgrade, previous: true, expectedStage: "construct deletable resource"},
		{name: "uninstall saved annotation", deployType: common.DeployTypeUninstall, previous: true, expectedStage: "construct deletable resource"},
		{name: "initial install new annotation", deployType: common.DeployTypeInitial, expectedStage: "construct installable resource"},
		{name: "upgrade new annotation", deployType: common.DeployTypeUpgrade, expectedStage: "construct installable resource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var previous, current []*spec.ResourceSpec
			if tt.previous {
				previous = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
					deleteDependencyAnnotation: "kind=Deployment,name=absent-target",
				})}
				if tt.deployType == common.DeployTypeUpgrade {
					current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
						deleteDependencyAnnotation: "state=absent,kind=Deployment,name=absent-target,external=false",
					})}
				}
			} else {
				current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
					deleteDependencyAnnotation: "kind=Deployment,version=v1,name=absent-target",
				})}
				if tt.deployType == common.DeployTypeUpgrade {
					previous = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)}
				}
			}

			var err error

			require.NotPanics(t, func() {
				_, _, err = buildTestResources(tt.deployType, previous, current)
			})
			require.ErrorContains(t, err, `"state" property must be set`)
			require.ErrorContains(t, err, deleteDependencyAnnotation)
			require.ErrorContains(t, err, tt.expectedStage)
		})
	}
}

func TestBuildResourcesMigratesIncompleteSavedDeleteDependency(t *testing.T) {
	correctedCharts := []struct {
		name string
		new  func(savedValue string) []*spec.ResourceSpec
	}{
		{
			name: "external=false",
			new: func(savedValue string) []*spec.ResourceSpec {
				return []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
					deleteDependencyAnnotation: savedValue + ",external=false",
				})}
			},
		},
		{
			name: "annotation removed",
			new: func(string) []*spec.ResourceSpec {
				return []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)}
			},
		},
		{
			name: "dependent resource removed",
			new: func(string) []*spec.ResourceSpec {
				return []*spec.ResourceSpec{defaultDeploymentResourceSpec(testReleaseNamespace)}
			},
		},
	}

	for _, saved := range incompleteSavedDeleteDependencies {
		for _, corrected := range correctedCharts {
			t.Run(saved.name+"/"+corrected.name, func(t *testing.T) {
				prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
					deleteDependencyAnnotation: saved.value,
				})}
				newSpecs := corrected.new(saved.value)

				_, delResources, err := buildTestResources(common.DeployTypeUpgrade, prev, newSpecs)
				require.NoError(t, err)

				dep := singleDeleteDependency(t, delResources, "dependent")
				require.False(t, dep.External)
				require.Equal(t, common.ResourceStateAbsent, dep.ResourceState)
				require.Equal(t, saved.value, prev[0].Annotations[deleteDependencyAnnotation])

				_, _, err = buildTestResources(common.DeployTypeUpgrade, newSpecs, newSpecs)
				require.NoError(t, err)

				_, _, err = buildTestResources(common.DeployTypeRollback, newSpecs, newSpecs)
				require.NoError(t, err)

				_, _, err = buildTestResources(common.DeployTypeUninstall, newSpecs, nil)
				require.NoError(t, err)
			})
		}
	}
}

func TestBuildResourcesNewDeleteDependencyStaysStrict(t *testing.T) {
	const incompleteValue = "state=absent,kind=Deployment,name=test-deployment"

	incompleteDependent := func() *spec.ResourceSpec {
		return newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
			deleteDependencyAnnotation: incompleteValue,
		})
	}

	tests := []struct {
		name       string
		deployType common.DeployType
		prev       []*spec.ResourceSpec
		new        []*spec.ResourceSpec
	}{
		{
			name:       "initial install",
			deployType: common.DeployTypeInitial,
			new:        []*spec.ResourceSpec{incompleteDependent()},
		},
		{
			name:       "unchanged incomplete annotation in new release",
			deployType: common.DeployTypeUpgrade,
			prev:       []*spec.ResourceSpec{incompleteDependent()},
			new:        []*spec.ResourceSpec{incompleteDependent()},
		},
		{
			name:       "target exists only in previous release",
			deployType: common.DeployTypeUpgrade,
			prev:       []*spec.ResourceSpec{incompleteDependent(), defaultDeploymentResourceSpec(testReleaseNamespace)},
			new:        []*spec.ResourceSpec{incompleteDependent()},
		},
		{
			name:       "rollback target contains incomplete annotation",
			deployType: common.DeployTypeRollback,
			prev:       []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)},
			new:        []*spec.ResourceSpec{incompleteDependent()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := buildTestResources(tt.deployType, tt.prev, tt.new)
			require.ErrorContains(t, err, "construct installable resource: validate delete dependencies")
			require.ErrorContains(t, err, `external delete dependency "target" must have "version" property set`)
		})
	}
}

func TestBuildResourcesSavedDeleteDependencyExternality(t *testing.T) {
	tests := []struct {
		name           string
		savedValue     string
		withPeer       bool
		wantErr        string
		wantExternal   bool
		wantMinMatches int
	}{
		{
			name:       "explicit external=true with incomplete target",
			savedValue: "state=absent,kind=Deployment,name=absent-target,external=true",
			wantErr:    `must have "version" property set`,
		},
		{
			name:       "explicit external=auto with incomplete target",
			savedValue: "state=absent,kind=Deployment,name=absent-target,external=auto",
			wantErr:    `must have "version" property set`,
		},
		{
			name:         "explicit external=false with incomplete target",
			savedValue:   "state=absent,kind=Deployment,name=absent-target,external=false",
			wantExternal: false,
		},
		{
			name:           "full target with omitted external",
			savedValue:     "state=absent,group=apps,version=v1,kind=Deployment,name=absent-target",
			wantExternal:   true,
			wantMinMatches: common.DefaultExternalDependencyMinMatches,
		},
		{
			name:         "incomplete target matching previous release peer",
			savedValue:   "state=absent,kind=Deployment,name=test-deployment",
			withPeer:     true,
			wantExternal: false,
		},
		{
			name:       "unknown property",
			savedValue: "state=absent,kind=Deployment,name=absent-target,unknown=value",
			wantErr:    `unknown property "unknown"`,
		},
		{
			name:       "invalid external value",
			savedValue: "state=absent,kind=Deployment,name=absent-target,external=maybe",
			wantErr:    `invalid value "maybe" for property "external"`,
		},
		{
			name:       "invalid state value",
			savedValue: "state=present,kind=Deployment,name=absent-target",
			wantErr:    `unknown value "present" for property "state"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				deleteDependencyAnnotation: tt.savedValue,
			})}
			if tt.withPeer {
				prev = append(prev, defaultDeploymentResourceSpec(testReleaseNamespace))
			}

			_, delResources, err := buildTestResources(common.DeployTypeUpgrade, prev, nil)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			dep := singleDeleteDependency(t, delResources, "dependent")
			require.Equal(t, tt.wantExternal, dep.External)
			require.Equal(t, tt.wantMinMatches, dep.MinMatches)
		})
	}
}

func TestBuildResourcesSavedDeleteDependencyWithTargetInNewRelease(t *testing.T) {
	dependent := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
		deleteDependencyAnnotation: "state=absent,kind=Deployment,name=test-deployment",
	})
	previous := []*spec.ResourceSpec{dependent}
	current := []*spec.ResourceSpec{dependent, defaultDeploymentResourceSpec(testReleaseNamespace)}

	_, _, err := buildTestResources(common.DeployTypeUpgrade, previous, current)
	require.NoError(t, err)

	_, _, err = buildTestResources(common.DeployTypeUpgrade, current, previous)
	require.ErrorContains(t, err, "construct installable resource: validate delete dependencies")
	require.ErrorContains(t, err, `external delete dependency "target" must have "version" property set`)
}

func TestBuildResourcesUninstallWithIncompleteSavedDeleteDependency(t *testing.T) {
	for _, saved := range incompleteSavedDeleteDependencies {
		t.Run(saved.name+"/regular resource", func(t *testing.T) {
			prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				deleteDependencyAnnotation: saved.value,
			})}

			_, delResources, err := buildTestResources(common.DeployTypeUninstall, prev, nil)
			require.NoError(t, err)
			require.False(t, singleDeleteDependency(t, delResources, "dependent").External)
		})

		t.Run(saved.name+"/pre-delete hook", func(t *testing.T) {
			prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				"helm.sh/hook":             "pre-delete",
				deleteDependencyAnnotation: saved.value,
			})}

			instResources, _, err := buildTestResources(common.DeployTypeUninstall, prev, nil)
			require.NoError(t, err)
			require.Len(t, instResources, 1)
			require.Equal(t, "dependent", instResources[0].Name)
		})
	}
}

func TestBuildResourcesWarnsOnIncompleteSavedDeleteDependencyFallback(t *testing.T) {
	tests := []struct {
		name       string
		savedValue string
		withPeer   bool
		wantWarn   bool
	}{
		{name: "incomplete target fallback", savedValue: "state=absent,kind=Deployment,name=absent-target", wantWarn: true},
		{name: "full external target", savedValue: "state=absent,group=apps,version=v1,kind=Deployment,name=absent-target"},
		{name: "incomplete target matching previous release peer", savedValue: "state=absent,kind=Deployment,name=test-deployment", withPeer: true},
		{name: "explicit external=false", savedValue: "state=absent,kind=Deployment,name=absent-target,external=false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				deleteDependencyAnnotation: tt.savedValue,
			})}
			if tt.withPeer {
				prev = append(prev, defaultDeploymentResourceSpec(testReleaseNamespace))
			}

			var out bytes.Buffer

			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))

			_, _, err := resource.BuildResources(ctx, common.DeployTypeUpgrade, testReleaseNamespace, prev, nil, nil, resource.BuildResourcesOptions{})
			require.NoError(t, err)

			warnings := lo.Filter(strings.Split(out.String(), "\n"), func(line string, _ int) bool {
				return strings.Contains(line, deleteDependencyAnnotation)
			})

			if !tt.wantWarn {
				require.Empty(t, warnings)

				return
			}

			require.Len(t, warnings, 1)
			require.Contains(t, warnings[0], "dependent")
			require.Contains(t, out.String(), "external=false")
		})
	}
}

func TestNewDeletableResourceWarnsAfterValidation(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(lo.Ternary(valid, "legacy fallback", "missing state"), func(t *testing.T) {
			value := "kind=Deployment,name=absent-target"
			if valid {
				value = "state=absent," + value
			}

			resSpec := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				deleteDependencyAnnotation: value,
			})

			var out bytes.Buffer

			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))

			res, err := resource.NewDeletableResource(ctx, resSpec, nil, testReleaseNamespace, resource.DeletableResourceOptions{})
			if !valid {
				require.ErrorContains(t, err, `"state" property must be set`)
				require.Empty(t, out.String())

				return
			}

			require.NoError(t, err)
			require.Len(t, res.ManualDependencies, 1)
			require.False(t, res.ManualDependencies[0].External)
			require.Equal(t, 1, strings.Count(out.String(), deleteDependencyAnnotation))
			require.Contains(t, out.String(), "dependent")
			require.Contains(t, out.String(), "external=false")
		})
	}
}
