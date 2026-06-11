//go:build ai_tests

package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

func TestAI_CopyReleaserWithStatus_UnexpectedTypeFails(t *testing.T) {
	_, err := CopyReleaserWithStatus("not-a-release", helmreleasecommon.StatusFailed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected release type")
}

func TestAI_CopyReleaserWithStatus_V1PreservesDescriptionAndOriginal(t *testing.T) {
	original := &helmrelease.Release{
		Name: "myrelease",
		Info: &helmrelease.Info{
			Status:      helmreleasecommon.StatusDeployed,
			Description: "original description",
		},
	}

	copied, err := CopyReleaserWithStatus(original, helmreleasecommon.StatusFailed)
	require.NoError(t, err)

	copiedRel, ok := copied.(*helmrelease.Release)
	require.True(t, ok)
	assert.Equal(t, helmreleasecommon.StatusFailed, copiedRel.Info.Status)
	assert.Equal(t, "original description", copiedRel.Info.Description)

	assert.Equal(t, helmreleasecommon.StatusDeployed, original.Info.Status, "original must not be mutated")
}

func TestAI_CopyReleaserWithStatus_V2PreservesDescriptionAndOriginal(t *testing.T) {
	original := &v2release.Release{
		Name: "myrelease",
		Info: &v2release.Info{
			Status:      helmreleasecommon.StatusDeployed,
			Description: "original description",
		},
	}

	copied, err := CopyReleaserWithStatus(original, helmreleasecommon.StatusFailed)
	require.NoError(t, err)

	copiedRel, ok := copied.(*v2release.Release)
	require.True(t, ok)
	assert.Equal(t, helmreleasecommon.StatusFailed, copiedRel.Info.Status)
	assert.Equal(t, "original description", copiedRel.Info.Description)

	assert.Equal(t, helmreleasecommon.StatusDeployed, original.Info.Status, "original must not be mutated")
}
