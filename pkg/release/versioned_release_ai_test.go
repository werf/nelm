//go:build ai_tests

package release

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v3chart "github.com/werf/nelm/pkg/helm/intern/chart/v3"
	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

func TestAI_VersionedRelease_NilRoundTrip(t *testing.T) {
	data, err := json.Marshal(&VersionedRelease{})
	require.NoError(t, err)
	assert.Equal(t, "null", string(data))

	var restored VersionedRelease
	require.NoError(t, json.Unmarshal([]byte("null"), &restored))
	assert.Nil(t, restored.Accessor)
}

func TestAI_VersionedRelease_RoundTripV1PreservesNativeType(t *testing.T) {
	original := &helmrelease.Release{
		Name: "myrelease",
		Info: &helmrelease.Info{
			Status: helmreleasecommon.StatusDeployed,
		},
		Chart: &v2chart.Chart{
			Metadata: &v2chart.Metadata{
				Name:       "mychart",
				Version:    "1.2.3",
				APIVersion: v2chart.APIVersionV2,
			},
		},
		Version:   3,
		Namespace: "myns",
	}

	acc, err := helmrel.NewAccessor(original)
	require.NoError(t, err)

	data, err := json.Marshal(&VersionedRelease{Accessor: acc})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version":"v1"`)

	var restored VersionedRelease
	require.NoError(t, json.Unmarshal(data, &restored))

	rel, ok := restored.Accessor.Releaser().(*helmrelease.Release)
	require.True(t, ok, "v1 release must round-trip as *helmrelease.Release, got %T", restored.Accessor.Releaser())
	assert.Equal(t, "myrelease", rel.Name)
	assert.Equal(t, 3, rel.Version)
	require.NotNil(t, rel.Chart)
	require.NotNil(t, rel.Chart.Metadata)
	assert.Equal(t, "mychart", rel.Chart.Metadata.Name)
}

func TestAI_VersionedRelease_RoundTripV2PreservesNativeType(t *testing.T) {
	original := &v2release.Release{
		Name: "myrelease",
		Info: &v2release.Info{
			Status:      helmreleasecommon.StatusDeployed,
			Description: "release deployed",
		},
		Chart: &v3chart.Chart{
			Metadata: &v3chart.Metadata{
				Name:       "mychart",
				Version:    "1.2.3",
				APIVersion: v3chart.APIVersionV3,
			},
		},
		Version:   7,
		Namespace: "myns",
	}

	acc, err := helmrel.NewAccessor(original)
	require.NoError(t, err)

	data, err := json.Marshal(&VersionedRelease{Accessor: acc})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version":"v2"`)

	var restored VersionedRelease
	require.NoError(t, json.Unmarshal(data, &restored))

	rel, ok := restored.Accessor.Releaser().(*v2release.Release)
	require.True(t, ok, "v2 release must round-trip as *v2release.Release, got %T", restored.Accessor.Releaser())
	assert.Equal(t, "myrelease", rel.Name)
	assert.Equal(t, 7, rel.Version)
	assert.Equal(t, "myns", rel.Namespace)
	assert.Equal(t, helmreleasecommon.StatusDeployed, rel.Info.Status)

	require.NotNil(t, rel.Chart)
	require.NotNil(t, rel.Chart.Metadata)
	assert.Equal(t, "mychart", rel.Chart.Metadata.Name)
	assert.Equal(t, "1.2.3", rel.Chart.Metadata.Version)
	assert.Equal(t, v3chart.APIVersionV3, rel.Chart.Metadata.APIVersion)
}

func TestAI_VersionedRelease_UnknownVersionFails(t *testing.T) {
	var restored VersionedRelease
	err := json.Unmarshal([]byte(`{"version":"v9","release":{}}`), &restored)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown release version")
}
