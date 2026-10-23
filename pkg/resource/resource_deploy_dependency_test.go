package resource_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

const deployDependencyAnnotation = "werf.io/deploy-dependency-target"

func TestBuildResourcesMigratesIncompleteSavedDeployDependency(t *testing.T) {
	for _, target := range []string{
		"group=apps,version=v1,kind=Deployment",
		"group=apps,version=v1,name=absent-target",
		"group=apps,kind=Deployment,name=absent-target",
	} {
		for _, state := range []string{"present", "ready"} {
			value := "state=" + state + "," + target
			for _, correction := range []string{"external=false", "annotation removed", "resource removed"} {
				t.Run(value+"/"+correction, func(t *testing.T) {
					prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
						deployDependencyAnnotation: value,
					})}

					var current []*spec.ResourceSpec
					switch correction {
					case "external=false":
						current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
							deployDependencyAnnotation: value + ",external=false",
						})}
					case "annotation removed":
						current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)}
					case "resource removed":
						current = []*spec.ResourceSpec{defaultDeploymentResourceSpec(testReleaseNamespace)}
					}

					_, _, err := buildTestResources(common.DeployTypeUpgrade, prev, current)
					require.NoError(t, err)
					require.Equal(t, value, prev[0].Annotations[deployDependencyAnnotation])

					_, _, err = buildTestResources(common.DeployTypeRollback, prev, current)
					require.NoError(t, err)

					_, _, err = buildTestResources(common.DeployTypeUninstall, prev, nil)
					require.NoError(t, err)
				})
			}
		}
	}
}

func TestBuildResourcesNewDeployDependencyFallsBackToInternal(t *testing.T) {
	const value = "state=ready,kind=Deployment,name=test-deployment"

	dependent := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
		deployDependencyAnnotation: value,
	})

	for _, deployType := range []common.DeployType{common.DeployTypeInitial, common.DeployTypeUpgrade, common.DeployTypeRollback} {
		t.Run(string(deployType), func(t *testing.T) {
			var prev []*spec.ResourceSpec
			if deployType != common.DeployTypeInitial {
				prev = []*spec.ResourceSpec{dependent, defaultDeploymentResourceSpec(testReleaseNamespace)}
			}

			resources, _, err := buildTestResources(deployType, prev, []*spec.ResourceSpec{dependent})
			require.NoError(t, err)
			require.Len(t, resources, 1)
			require.Len(t, resources[0].ManualDependencies, 1)
			require.False(t, resources[0].ManualDependencies[0].External)
		})
	}

	_, _, err := buildTestResources(common.DeployTypeUpgrade, []*spec.ResourceSpec{dependent}, []*spec.ResourceSpec{dependent})
	require.NoError(t, err)
}

func TestBuildResourcesSavedDeployDependencyExternality(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		withPeer     bool
		wantExternal bool
		wantWarn     bool
		wantErr      string
	}{
		{name: "missing name", value: "state=ready,group=apps,version=v1,kind=Deployment", wantWarn: true},
		{name: "missing kind", value: "state=ready,group=apps,version=v1,name=absent-target", wantWarn: true},
		{name: "missing version", value: "state=ready,kind=Deployment,name=absent-target", wantWarn: true},
		{name: "explicit true", value: "state=ready,kind=Deployment,name=absent-target,external=true", wantErr: `must have "version" property set`},
		{name: "explicit auto", value: "state=ready,kind=Deployment,name=absent-target,external=auto", wantErr: `must have "version" property set`},
		{name: "explicit false", value: "state=ready,kind=Deployment,name=absent-target,external=false"},
		{name: "full external target", value: "state=ready,group=apps,version=v1,kind=Deployment,name=absent-target", wantExternal: true},
		{name: "previous release peer", value: "state=ready,kind=Deployment,name=test-deployment,namespace=" + testReleaseNamespace, withPeer: true},
		{name: "missing state", value: "kind=Deployment,name=absent-target", wantErr: `"state" property must be set`},
		{name: "unknown property", value: "state=ready,kind=Deployment,name=absent-target,unknown=value", wantErr: `unknown property "unknown"`},
		{name: "invalid external", value: "state=ready,kind=Deployment,name=absent-target,external=maybe", wantErr: `invalid value "maybe" for property "external"`},
		{name: "invalid state", value: "state=absent,kind=Deployment,name=absent-target", wantErr: `unknown value "absent" for property "state"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				"helm.sh/hook":             "pre-delete",
				deployDependencyAnnotation: tt.value,
			})}
			if tt.withPeer {
				prev = append(prev, defaultDeploymentResourceSpec(testReleaseNamespace))
			}

			var out bytes.Buffer

			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))

			_, _, err := resource.BuildResources(ctx, common.DeployTypeUpgrade, testReleaseNamespace, prev, nil, nil, resource.BuildResourcesOptions{})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, out.String())

				return
			}

			require.NoError(t, err)

			var expectedWarnings int
			if tt.wantWarn {
				expectedWarnings = 1

				require.Contains(t, out.String(), "dependent")
				require.Contains(t, out.String(), "external=false")
			}

			require.Equal(t, expectedWarnings, strings.Count(out.String(), deployDependencyAnnotation))
			out.Reset()

			instResources, _, err := resource.BuildResources(ctx, common.DeployTypeUninstall, testReleaseNamespace, prev, nil, nil, resource.BuildResourcesOptions{})
			require.NoError(t, err)
			require.Equal(t, expectedWarnings, strings.Count(out.String(), deployDependencyAnnotation))
			require.Len(t, instResources, 1)
			require.Len(t, instResources[0].ManualDependencies, 1)

			dep := instResources[0].ManualDependencies[0]
			require.Equal(t, tt.wantExternal, dep.External)
			require.Equal(t, common.ResourceStateReady, dep.ResourceState)

			if tt.wantExternal {
				require.Equal(t, common.DefaultExternalDependencyMinMatches, dep.MinMatches)
				require.Equal(t, common.DefaultExternalDependencyMaxMatches, dep.MaxMatches)
			} else {
				require.Zero(t, dep.MinMatches)
				require.Zero(t, dep.MaxMatches)
			}

			require.Equal(t, tt.value, prev[0].Annotations[deployDependencyAnnotation])
		})
	}
}
