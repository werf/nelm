package release

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestSQLStorageBackend_AnnotationWritePermissionHintPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	b.db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	namespace := "nelm-test-permission-" + suffix
	role := "nelm_annotation_denied_" + suffix
	t.Cleanup(func() {
		for _, query := range []string{
			"RESET ROLE",
			"DELETE FROM custom_labels_v1 WHERE releaseNamespace = " + pq.QuoteLiteral(namespace),
			"DELETE FROM releases_v1 WHERE namespace = " + pq.QuoteLiteral(namespace),
			"DROP POLICY IF EXISTS " + pq.QuoteIdentifier(role) + " ON releases_v1",
			"DROP POLICY IF EXISTS " + pq.QuoteIdentifier(role) + " ON custom_labels_v1",
			"DROP ROLE IF EXISTS " + pq.QuoteIdentifier(role),
		} {
			_, err := b.db.ExecContext(ctx, query)
			assert.NoError(t, err)
		}
	})

	storage := newReleaseStorage(namespace, b, 0)
	existing := newTestRelease(namespace, "existing", 1, common.StatusDeployed)
	existing.Info.Annotations = map[string]string{"team": "platform"}
	existingAccessor, err := helmrel.NewAccessor(existing)
	require.NoError(t, err)
	require.NoError(t, storage.Create(ctx, existingAccessor))

	for _, query := range []string{
		"CREATE ROLE " + pq.QuoteIdentifier(role),
		"CREATE POLICY " + pq.QuoteIdentifier(role) + " ON releases_v1 TO " + pq.QuoteIdentifier(role) + " USING (namespace = " + pq.QuoteLiteral(namespace) + ") WITH CHECK (namespace = " + pq.QuoteLiteral(namespace) + ")",
		"CREATE POLICY " + pq.QuoteIdentifier(role) + " ON custom_labels_v1 TO " + pq.QuoteIdentifier(role) + " USING (releaseNamespace = " + pq.QuoteLiteral(namespace) + ") WITH CHECK (releaseNamespace = " + pq.QuoteLiteral(namespace) + ")",
		"SET ROLE " + pq.QuoteIdentifier(role),
	} {
		_, err = b.db.ExecContext(ctx, query)
		require.NoError(t, err)
	}

	rl := newTestRelease(namespace, "denied", 1, common.StatusPendingInstall)
	rl.Labels = map[string]string{"team": "platform"}
	rl.Info.Annotations = map[string]string{"team": "platform"}
	accessor, err := helmrel.NewAccessor(rl)
	require.NoError(t, err)
	err = storage.Create(ctx, accessor)

	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr)
	assert.Equal(t, pqErr.Code, pqerror.InsufficientPrivilege)

	for _, hint := range []string{"insert annotations:", sqlAnnotationsTable, namespace, "check", "current role", "table privileges", "row-level security (RLS) policies"} {
		require.ErrorContains(t, err, hint)
	}

	stored, err := b.get(ctx, namespace, storageKey("existing", 1))
	require.NoError(t, err)
	require.Nil(t, stored.Annotations, "annotation rows are hidden without an annotation policy")

	loaded, err := storage.GetRelease(ctx, "existing", 1)
	require.NoError(t, err)
	assert.Equal(t, "platform", loaded.Annotations()["team"])

	_, err = b.db.ExecContext(ctx, "RESET ROLE")
	require.NoError(t, err)

	var count int
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM releases_v1 WHERE namespace = $1 AND key = $2", namespace, storageKey("denied", 1)))
	assert.Zero(t, count)
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM custom_labels_v1 WHERE releaseNamespace = $1 AND releaseKey = $2", namespace, storageKey("denied", 1)))
	assert.Zero(t, count)
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1 AND releaseKey = $2", namespace, storageKey("denied", 1)))
	assert.Zero(t, count)
}
