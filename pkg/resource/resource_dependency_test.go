package resource_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

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
						_, _, err = resource.BuildResources(ctx, common.DeployTypeUpgrade, testReleaseNamespace, prev, nil, nil, resource.BuildResourcesOptions{})
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
