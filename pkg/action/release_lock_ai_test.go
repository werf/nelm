//go:build ai_tests

package action

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/werf/lockgate"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/kube/fake"
)

const (
	lockTestReleaseName      = "myrelease"
	lockTestReleaseNamespace = "test-namespace"
)

var lockConfigMapGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

// Exercises newReleaseLockManager, the seam all three release actions use to gate locking behind
// LegacyNoReleaseLock: a nil manager means the caller skips acquiring the lock entirely.
func TestAI_ReleaseLockAcquiredWhenLegacyNoReleaseLockDisabled(t *testing.T) {
	ctx := context.Background()

	clientFactory, err := fake.NewClientFactory(ctx)
	require.NoError(t, err)

	require.False(t, lockConfigMapExists(t, ctx, clientFactory), "lock ConfigMap must not exist before locking")

	lockManager, lockEnabled, err := newReleaseLockManager(ctx, lockTestReleaseNamespace, clientFactory, false)
	require.NoError(t, err)
	require.True(t, lockEnabled, "locking must be reported enabled")
	require.NotNil(t, lockManager, "lock manager must be constructed when locking is enabled")

	handle, err := lockManager.LockRelease(ctx, lockTestReleaseName)
	require.NoError(t, err)

	assert.True(t, lockConfigMapExists(t, ctx, clientFactory), "acquiring the release lock must create the lock ConfigMap")

	require.NoError(t, lockManager.Unlock(handle))
}

func TestAI_ReleaseLockSkippedWhenLegacyNoReleaseLockEnabled(t *testing.T) {
	ctx := context.Background()

	clientFactory, err := fake.NewClientFactory(ctx)
	require.NoError(t, err)

	lockManager, lockEnabled, err := newReleaseLockManager(ctx, lockTestReleaseNamespace, clientFactory, true)
	require.NoError(t, err)
	assert.False(t, lockEnabled, "locking must be reported disabled so callers skip acquiring")
	assert.Nil(t, lockManager, "lock manager must not be constructed when locking is disabled")

	assert.False(t, lockConfigMapExists(t, ctx, clientFactory), "disabling the release lock must not touch the lock ConfigMap")
}

func TestAI_ReleaseLockIsExclusiveWhenLegacyNoReleaseLockDisabled(t *testing.T) {
	ctx := context.Background()

	clientFactory, err := fake.NewClientFactory(ctx)
	require.NoError(t, err)

	lockManager, _, err := newReleaseLockManager(ctx, lockTestReleaseNamespace, clientFactory, false)
	require.NoError(t, err)
	require.NotNil(t, lockManager)

	handle, err := lockManager.LockRelease(ctx, lockTestReleaseName)
	require.NoError(t, err)

	acquired, _, err := lockManager.LockerWithRetry.Acquire(
		"release/"+lockTestReleaseName,
		lockgate.AcquireOptions{NonBlocking: true},
	)
	require.NoError(t, err)
	assert.False(t, acquired, "a held release lock must not be granted twice")

	require.NoError(t, lockManager.Unlock(handle))

	acquired, secondHandle, err := lockManager.LockerWithRetry.Acquire(
		"release/"+lockTestReleaseName,
		lockgate.AcquireOptions{NonBlocking: true},
	)
	require.NoError(t, err)
	assert.True(t, acquired, "the release lock must be grantable again after unlock")

	require.NoError(t, lockManager.Unlock(secondHandle))
}

func lockConfigMapExists(t *testing.T, ctx context.Context, clientFactory kube.ClientFactorier) bool {
	t.Helper()

	_, err := clientFactory.Dynamic().
		Resource(lockConfigMapGVR).
		Namespace(lockTestReleaseNamespace).
		Get(ctx, common.LockConfigMapName, metav1.GetOptions{})
	if err != nil {
		require.True(t, apierrors.IsNotFound(err), "unexpected error looking up the lock ConfigMap: %v", err)

		return false
	}

	return true
}
