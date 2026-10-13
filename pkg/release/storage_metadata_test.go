package release

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseStorage_VersionReadsDoNotKeepAnnotations(t *testing.T) {
	ctx := context.Background()
	rls := newTestReleaseWithStatus("app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "platform"}
	storage := newMemoryReleaseStorage(t, rls)

	revisions, err := storage.readableRevisions(ctx, testNamespace, "app")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	assert.Nil(t, revisions[0].Annotations)

	revisions, err = storage.Revisions(ctx, "app")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	assert.Equal(t, "platform", revisions[0].Annotations["team"])
	assert.Equal(t, storageAnnotationReleasedByValue, revisions[0].Annotations[storageAnnotationReleasedBy])

	loaded, err := storage.GetRelease(ctx, "app", 0)
	require.NoError(t, err)
	assert.Equal(t, revisions[0].Annotations, loaded.Annotations())
}
