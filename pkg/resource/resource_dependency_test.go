package resource_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestBuildResourcesDependencyFallbackForAllResources(t *testing.T) {
	for _, dependency := range []struct {
		annotation string
		state      string
	}{
		{annotation: deployDependencyAnnotation, state: "ready"},
		{annotation: deleteDependencyAnnotation, state: "absent"},
	} {
		for _, deployType := range []common.DeployType{
			common.DeployTypeInitial, common.DeployTypeInstall, common.DeployTypeUpgrade, common.DeployTypeRollback, common.DeployTypeUninstall,
		} {
			for _, tt := range []struct {
				name         string
				value        string
				wantWarn     bool
				wantExternal bool
				wantErr      string
			}{
				{name: "missing name", value: "group=apps,version=v1,kind=Deployment", wantWarn: true},
				{name: "missing kind", value: "group=apps,version=v1,name=target", wantWarn: true},
				{name: "missing version", value: "group=apps,kind=Deployment,name=target", wantWarn: true},
				{name: "complete target", value: "group=apps,version=v1,kind=Deployment,name=target", wantExternal: true},
				{name: "explicit internal", value: "kind=Deployment,name=target,external=false"},
				{name: "explicit external", value: "kind=Deployment,name=target,external=true", wantErr: `must have "version" property set`},
				{name: "explicit auto", value: "kind=Deployment,name=target,external=auto", wantErr: `must have "version" property set`},
				{name: "invalid external", value: "kind=Deployment,name=target,external=invalid", wantErr: `invalid value "invalid" for property "external"`},
				{name: "unknown property", value: "kind=Deployment,name=target,unknown=value", wantErr: `unknown property "unknown"`},
			} {
				t.Run(dependency.annotation+"/"+string(deployType)+"/"+tt.name, func(t *testing.T) {
					value := "state=" + dependency.state + "," + tt.value
					resSpec := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{dependency.annotation: value})
					current := []*spec.ResourceSpec{resSpec}

					var previous []*spec.ResourceSpec
					switch deployType {
					case common.DeployTypeUninstall:
						previous, current = current, nil
					case common.DeployTypeUpgrade, common.DeployTypeRollback:
						previous = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)}
					}

					var out bytes.Buffer

					ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))

					_, _, err := resource.BuildResources(ctx, deployType, testReleaseNamespace, previous, current, nil, resource.BuildResourcesOptions{})
					if tt.wantErr != "" {
						require.ErrorContains(t, err, tt.wantErr)
						require.Empty(t, out.String())

						return
					}

					require.NoError(t, err)

					var deps []*resource.Dependency
					if dependency.annotation == deployDependencyAnnotation {
						res, err := resource.NewInstallableResource(ctx, resSpec, nil, testReleaseNamespace, resource.InstallableResourceOptions{})
						require.NoError(t, err)

						deps = res.ManualDependencies
					} else {
						res, err := resource.NewDeletableResource(ctx, resSpec, nil, testReleaseNamespace, resource.DeletableResourceOptions{})
						require.NoError(t, err)

						deps = res.ManualDependencies
					}

					require.Len(t, deps, 1)
					require.Equal(t, tt.wantExternal, deps[0].External)
					require.Equal(t, value, resSpec.Annotations[dependency.annotation])

					if tt.wantWarn {
						require.Equal(t, 1, strings.Count(out.String(), dependency.annotation))
						require.Contains(t, out.String(), "treated as internal")
						require.NotContains(t, out.String(), "previous release")
					} else {
						require.Empty(t, out.String())
					}
				})
			}
		}
	}
}

func TestBuildResourcesDependencyFallbackWarningsAreNotDuplicated(t *testing.T) {
	for _, annotation := range []string{deployDependencyAnnotation, deleteDependencyAnnotation} {
		t.Run(annotation, func(t *testing.T) {
			state := "ready"
			if annotation == deleteDependencyAnnotation {
				state = "absent"
			}

			annotations := map[string]string{annotation: "state=" + state + ",kind=Deployment,name=target"}
			previous := newDependentConfigMapSpec(testReleaseNamespace, annotations)
			current := newDependentConfigMapSpec(testReleaseNamespace, annotations)

			var out bytes.Buffer

			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))
			_, _, err := resource.BuildResources(ctx, common.DeployTypeUpgrade, testReleaseNamespace,
				[]*spec.ResourceSpec{previous}, []*spec.ResourceSpec{current}, nil, resource.BuildResourcesOptions{})
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(out.String(), annotation))
		})
	}
}

func TestBuildResourcesDependencyWarningsUseRelevantRevisions(t *testing.T) {
	for _, deployType := range []common.DeployType{common.DeployTypeUpgrade, common.DeployTypeRollback} {
		for _, correction := range []string{"external=false", "annotation removed", "resource removed", "target added", "unchanged"} {
			t.Run(string(deployType)+"/"+correction, func(t *testing.T) {
				annotations := map[string]string{
					deployDependencyAnnotation: "state=ready,kind=Deployment,name=test-deployment",
					deleteDependencyAnnotation: "state=absent,kind=Deployment,name=test-deployment",
				}
				previous := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, annotations)}

				var current []*spec.ResourceSpec
				switch correction {
				case "external=false":
					current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
						deployDependencyAnnotation: annotations[deployDependencyAnnotation] + ",external=false",
						deleteDependencyAnnotation: annotations[deleteDependencyAnnotation] + ",external=false",
					})}
				case "annotation removed":
					current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, nil)}
				case "resource removed":
					current = []*spec.ResourceSpec{defaultDeploymentResourceSpec(testReleaseNamespace)}
				case "target added":
					current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, annotations), defaultDeploymentResourceSpec(testReleaseNamespace)}
				case "unchanged":
					current = []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, annotations)}
				}

				var out bytes.Buffer

				ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))
				_, _, err := resource.BuildResources(ctx, deployType, testReleaseNamespace, previous, current, nil, resource.BuildResourcesOptions{})
				require.NoError(t, err)

				var deployWarnings int
				if correction == "unchanged" {
					deployWarnings = 1
				}

				require.Equal(t, deployWarnings, strings.Count(out.String(), deployDependencyAnnotation))
				require.Equal(t, 1, strings.Count(out.String(), deleteDependencyAnnotation))
			})
		}
	}
}

func TestBuildResourcesDependencyWarningsWithCanceledContext(t *testing.T) {
	for _, dependency := range []struct {
		annotation string
		state      string
	}{
		{annotation: deployDependencyAnnotation, state: "ready"},
		{annotation: deleteDependencyAnnotation, state: "absent"},
	} {
		for _, contextState := range []string{"canceled", "deadline exceeded"} {
			for _, tt := range []struct {
				name     string
				value    string
				wantWarn bool
				wantErr  string
			}{
				{name: "legacy fallback", value: "kind=Deployment,name=target", wantWarn: true},
				{name: "complete external target", value: "group=apps,version=v1,kind=Deployment,name=target"},
				{name: "explicit internal", value: "kind=Deployment,name=target,external=false"},
				{name: "explicit external", value: "kind=Deployment,name=target,external=true", wantErr: `must have "version" property set`},
				{name: "explicit auto", value: "kind=Deployment,name=target,external=auto", wantErr: `must have "version" property set`},
			} {
				t.Run(dependency.annotation+"/"+contextState+"/"+tt.name, func(t *testing.T) {
					var out bytes.Buffer

					logCtx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))

					var (
						ctx    context.Context
						cancel context.CancelFunc
					)
					if contextState == "canceled" {
						ctx, cancel = context.WithCancel(logCtx)
						cancel()
						require.ErrorIs(t, ctx.Err(), context.Canceled)
					} else {
						ctx, cancel = context.WithDeadline(logCtx, time.Unix(0, 0))
						require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
					}

					t.Cleanup(cancel)

					prev := []*spec.ResourceSpec{newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
						dependency.annotation: "state=" + dependency.state + "," + tt.value,
					})}

					var err error

					require.NotPanics(t, func() {
						_, _, err = resource.BuildResources(ctx, common.DeployTypeUpgrade, testReleaseNamespace, prev, prev, nil, resource.BuildResourcesOptions{})
					})

					if tt.wantErr != "" {
						require.ErrorContains(t, err, tt.wantErr)
						require.Empty(t, out.String())

						return
					}

					require.NoError(t, err)

					if tt.wantWarn {
						require.Contains(t, out.String(), dependency.annotation)
						require.Contains(t, out.String(), "treated as internal")
					} else {
						require.Empty(t, out.String())
					}
				})
			}
		}
	}
}

func TestBuildResourcesWarnsOnPatchedDependencyFallback(t *testing.T) {
	resSpec := newDependentConfigMapSpec(testReleaseNamespace, nil)
	patcher := spec.NewExtraMetadataPatcher(map[string]string{
		deployDependencyAnnotation: "state=ready,kind=Deployment,name=target",
	}, nil)

	var out bytes.Buffer

	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&out, &out))
	resources, _, err := resource.BuildResources(ctx, common.DeployTypeInitial, testReleaseNamespace, nil,
		[]*spec.ResourceSpec{resSpec}, []spec.ResourcePatcher{patcher}, resource.BuildResourcesOptions{})
	require.NoError(t, err)
	require.Len(t, resources, 1)
	require.Len(t, resources[0].ManualDependencies, 1)
	require.False(t, resources[0].ManualDependencies[0].External)
	require.Equal(t, 1, strings.Count(out.String(), deployDependencyAnnotation))
	require.NotContains(t, resSpec.Annotations, deployDependencyAnnotation)
}
