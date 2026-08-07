package release_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/pkg/release"
)

func TestIsReleaseUpToDate(t *testing.T) {
	cmManifest := func(dataVal string) string {
		return `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: ` + dataVal + "\n"
	}

	baseOldRel := func() *helmrelease.Release {
		return &helmrelease.Release{
			Info:     &helmrelease.Info{Status: helmreleasecommon.StatusDeployed},
			Config:   map[string]interface{}{},
			Manifest: cmManifest("value1"),
			Hooks:    []*helmrelease.Hook{{Manifest: cmManifest("hookval1")}},
		}
	}

	baseNewRel := func() *helmrelease.Release {
		return &helmrelease.Release{
			Info:     &helmrelease.Info{Status: helmreleasecommon.StatusDeployed},
			Config:   map[string]interface{}{},
			Manifest: cmManifest("value1"),
			Hooks:    []*helmrelease.Hook{{Manifest: cmManifest("hookval1")}},
		}
	}

	tests := []struct {
		name             string
		oldRel           *helmrelease.Release
		newRel           *helmrelease.Release
		expectedUpToDate bool
		expectedReason   release.ReleaseOutdatedReason
	}{
		{
			name:             "up-to-date",
			oldRel:           baseOldRel(),
			newRel:           baseNewRel(),
			expectedUpToDate: true,
			expectedReason:   release.ReleaseOutdatedReasonNone,
		},
		{
			name:             "no-previous-release",
			oldRel:           nil,
			newRel:           baseNewRel(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonNoPreviousRelease,
		},
		{
			name: "status-not-deployed",
			oldRel: func() *helmrelease.Release {
				r := baseOldRel()
				r.Info.Status = helmreleasecommon.StatusFailed

				return r
			}(),
			newRel:           baseNewRel(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonReleaseStatusNotDeployed,
		},
		{
			name: "notes-changed",
			oldRel: func() *helmrelease.Release {
				r := baseOldRel()
				r.Info.Notes = "old"

				return r
			}(),
			newRel: func() *helmrelease.Release {
				r := baseNewRel()
				r.Info.Notes = "new"

				return r
			}(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonNotesChanged,
		},
		{
			name: "values-changed",
			oldRel: func() *helmrelease.Release {
				r := baseOldRel()
				r.Config = map[string]interface{}{"a": 1}

				return r
			}(),
			newRel: func() *helmrelease.Release {
				r := baseNewRel()
				r.Config = map[string]interface{}{"a": 2}

				return r
			}(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonValuesChanged,
		},
		{
			name: "hooks-changed",
			oldRel: func() *helmrelease.Release {
				r := baseOldRel()
				r.Hooks = []*helmrelease.Hook{{Manifest: cmManifest("hookval1")}}

				return r
			}(),
			newRel: func() *helmrelease.Release {
				r := baseNewRel()
				r.Hooks = []*helmrelease.Hook{{Manifest: cmManifest("hookval2")}}

				return r
			}(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonHooksChanged,
		},
		{
			name: "manifests-changed",
			oldRel: func() *helmrelease.Release {
				r := baseOldRel()
				r.Hooks = nil
				r.Manifest = cmManifest("value1")

				return r
			}(),
			newRel: func() *helmrelease.Release {
				r := baseNewRel()
				r.Hooks = nil
				r.Manifest = cmManifest("value2")

				return r
			}(),
			expectedUpToDate: false,
			expectedReason:   release.ReleaseOutdatedReasonManifestsChanged,
		},
	}

	accessor := func(t *testing.T, rel *helmrelease.Release) helmrel.Accessor {
		t.Helper()

		if rel == nil {
			return nil
		}

		acc, err := helmrel.NewAccessor(rel)
		require.NoError(t, err)

		return acc
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := release.IsReleaseUpToDate(accessor(t, tt.oldRel), accessor(t, tt.newRel))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedUpToDate, result.UpToDate)
			assert.Equal(t, tt.expectedReason, result.Reason)
		})
	}
}
