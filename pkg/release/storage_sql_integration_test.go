package release

import (
	"context"
	"maps"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/storage/driver"
)

func TestSQLStorageBackend_AnnotationPermissionsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	prefix := "nelm-test-rls-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	for _, namespace := range []string{prefix, prefix + "-hidden"} {
		obj := newTestStoredObject(t, newTestRelease(namespace, "app", 1, common.StatusDeployed))
		obj.Annotations = map[string]string{"nelm.werf.io/released-by": "nelm", "secret": namespace}
		require.NoError(t, b.create(ctx, obj))
		t.Cleanup(func() { require.NoError(t, b.delete(ctx, namespace, obj.Key)) })
	}

	tx, err := b.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { rollbackSQLTransaction(ctx, tx) })

	role := "nelm_annotations_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = tx.ExecContext(ctx, "CREATE ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON releases_v1 TO "+pq.QuoteIdentifier(role)+" USING (namespace = "+pq.QuoteLiteral(prefix)+")")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)

	var annotationCount int
	require.NoError(t, tx.GetContext(ctx, &annotationCount, "SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1", prefix))
	require.Zero(t, annotationCount, "release access alone must not grant annotation access")

	_, err = tx.ExecContext(ctx, "RESET ROLE")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON nelm_release_annotations_v1 TO "+pq.QuoteIdentifier(role)+" USING (releaseNamespace = "+pq.QuoteLiteral(prefix)+") WITH CHECK (releaseNamespace = "+pq.QuoteLiteral(prefix)+")")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)

	var migrationIDs []string
	require.NoError(t, tx.SelectContext(ctx, &migrationIDs, "SELECT id FROM nelm_migrations ORDER BY id"))
	require.Equal(t, []string{"release_annotations_v1"}, migrationIDs)

	_, err = tx.ExecContext(ctx, "SAVEPOINT truncate_check")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "TRUNCATE nelm_release_annotations_v1")
	require.ErrorContains(t, err, "permission denied")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT truncate_check")
	require.NoError(t, err)

	var namespaces []string
	require.NoError(t, tx.SelectContext(ctx, &namespaces, "SELECT DISTINCT releaseNamespace FROM nelm_release_annotations_v1 WHERE releaseNamespace IN ($1, $2)", prefix, prefix+"-hidden"))
	require.Equal(t, []string{prefix}, namespaces)

	var count int
	require.NoError(t, tx.GetContext(ctx, &count, "SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1", prefix+"-hidden"))
	require.Zero(t, count)

	_, err = tx.ExecContext(ctx, "SAVEPOINT move_annotation")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE nelm_release_annotations_v1 SET releaseNamespace = $1 WHERE releaseNamespace = $2", prefix+"-hidden", prefix)
	require.ErrorContains(t, err, "row-level security policy")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT move_annotation")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "INSERT INTO nelm_release_annotations_v1 (releaseKey, releaseNamespace, key, value, releaseModifiedAt) VALUES ($1, $2, 'leak', 'value', 0)", storageKey("app", 1), prefix+"-hidden")
	require.ErrorContains(t, err, "row-level security policy")
	require.NoError(t, tx.Rollback())
}

func TestSQLStorageBackend_AnnotationStorageInteropPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })
	require.NoError(t, ensureSQLStorageSchema(ctx, b.db))
	require.NoError(t, ensureSQLStorageSchema(ctx, b.db))

	var helmMigrations, nelmMigrations []string
	require.NoError(t, b.db.SelectContext(ctx, &helmMigrations, "SELECT id FROM gorp_migrations ORDER BY id"))
	require.Equal(t, []string{"custom_labels", "init"}, helmMigrations)
	require.NoError(t, b.db.SelectContext(ctx, &nelmMigrations, "SELECT id FROM nelm_migrations ORDER BY id"))
	require.Equal(t, []string{"release_annotations_v1"}, nelmMigrations)

	namespace := "nelm-test-interop-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	storage := newReleaseStorage(namespace, b, 0)
	rls := newTestRelease(namespace, "app", 1, common.StatusPendingInstall)
	rls.Info.Annotations = map[string]string{"first": "value", "removed": "yes"}
	accessor, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, storage.Create(ctx, accessor))
	t.Cleanup(func() { require.NoError(t, storage.Delete(ctx, "app", 1)) })
	require.Equal(t, map[string]string{"first": "value", "removed": "yes"}, rls.Info.Annotations)

	stored, err := b.get(ctx, namespace, storageKey("app", 1))
	require.NoError(t, err)
	body, err := decodeRelease(stored.Body)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, body.Info.Annotations)
	require.Equal(t, map[string]string{"nelm.werf.io/released-by": "nelm", "first": "value", "removed": "yes"}, stored.Annotations)

	loaded, err := storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, loaded.Annotations())

	revisions, err := storage.Revisions(ctx, "app")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.Equal(t, stored.Annotations, revisions[0].Annotations)

	latest, err := storage.LatestRevisions(ctx, LatestRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.Equal(t, stored.Annotations, latest[0].Annotations)

	helm, err := driver.NewSQL(connection, namespace)
	require.NoError(t, err)
	helmRelease, err := helm.Get(storageKey("app", 1))
	require.NoError(t, err)
	accessor, err = helmrel.NewAccessor(helmRelease)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, accessor.Annotations())

	loaded.SetStatus(common.StatusDeployed)
	require.NoError(t, storage.Update(ctx, loaded))
	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, loaded.Annotations())

	_, err = b.db.ExecContext(ctx, "UPDATE releases_v1 SET modifiedAt = 1 WHERE namespace = $1 AND key = $2", namespace, storageKey("app", 1))
	require.NoError(t, err)
	_, err = b.db.ExecContext(ctx, "UPDATE nelm_release_annotations_v1 SET releaseModifiedAt = 1 WHERE releaseNamespace = $1 AND releaseKey = $2", namespace, storageKey("app", 1))
	require.NoError(t, err)
	olderRelease, err := helm.Get(storageKey("app", 1))
	require.NoError(t, err)
	olderAccessor, err := helmrel.NewAccessor(olderRelease)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, olderAccessor.Annotations())
	olderAccessor.Annotations()["first"] = "older"
	delete(olderAccessor.Annotations(), "removed")
	olderAccessor.Annotations()["extra"] = "value"
	olderAccessor.SetStatus(common.StatusSuperseded)
	require.NoError(t, helm.Update(storageKey("app", 1), olderRelease))
	stored, err = b.get(ctx, namespace, storageKey("app", 1))
	require.NoError(t, err)
	require.Nil(t, stored.Annotations)

	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, "superseded", loaded.Status())
	require.Equal(t, olderAccessor.Annotations(), loaded.Annotations())
	require.Equal(t, "older", loaded.Annotations()["first"])
	require.NotContains(t, loaded.Annotations(), "removed")
	require.Equal(t, "value", loaded.Annotations()["extra"])

	summary, err := storage.LoadRevisionSummary(ctx, Revision{Name: "app", Namespace: namespace, Version: 1})
	require.NoError(t, err)
	require.Equal(t, olderAccessor.Annotations(), summary.Annotations)

	updated, err := ReleaserToV1Release(loaded.Releaser())
	require.NoError(t, err)

	updated.Info.Annotations = map[string]string{"first": "new"}
	accessor, err = helmrel.NewAccessor(updated)
	require.NoError(t, err)
	require.NoError(t, storage.Update(ctx, accessor))
	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"nelm.werf.io/released-by": "nelm", "first": "new"}, loaded.Annotations())
	updated.Info.Annotations = nil
	accessor, err = helmrel.NewAccessor(updated)
	require.NoError(t, err)
	require.NoError(t, storage.Update(ctx, accessor))
	stored, err = b.get(ctx, namespace, storageKey("app", 1))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"nelm.werf.io/released-by": "nelm"}, stored.Annotations)

	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, stored.Annotations, loaded.Annotations())
}

func TestSQLStorageBackend_AnnotationsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })
	b.db.SetMaxOpenConns(1)

	namespace := "nelm-test-annotations-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	rls := newTestRelease(namespace, "app", 1, common.StatusDeployed)
	obj := newTestStoredObject(t, rls)
	annotations := map[string]string{"nelm.werf.io/released-by": "nelm", strings.Repeat("arbitrary key / '", 30): strings.Repeat("value", 1000), "removed": "yes"}
	obj.Annotations = maps.Clone(annotations)
	require.NoError(t, b.create(ctx, obj))
	t.Cleanup(func() {
		_, err := b.db.ExecContext(ctx, "DELETE FROM releases_v1 WHERE namespace = $1", namespace)
		require.NoError(t, err)
	})

	stored, err := b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, annotations, stored.Annotations)

	var mismatched int
	require.NoError(t, b.db.GetContext(ctx, &mismatched, `SELECT count(*) FROM nelm_release_annotations_v1 a JOIN releases_v1 r ON r.key = a.releaseKey AND r.namespace = a.releaseNamespace WHERE r.namespace = $1 AND a.releaseModifiedAt <> COALESCE(r.modifiedAt, 0)`, namespace))
	require.Zero(t, mismatched)

	metadata, err := b.listMetadata(ctx, namespace, "app", true)
	require.NoError(t, err)
	require.Len(t, metadata, 1)
	require.Equal(t, annotations, metadata[0].Annotations)
	require.Nil(t, metadata[0].Body)

	for _, withBodies := range []bool{false, true} {
		calls := 0
		require.NoError(t, b.scanLatestCandidates(ctx, namespace, nil, withBodies, func(obj *storedObject) error {
			calls++

			require.Equal(t, annotations, obj.Annotations)
			require.Equal(t, withBodies, len(obj.Body) > 0)

			return nil
		}))
		require.Equal(t, 1, calls)
	}

	calls := 0
	require.NoError(t, b.listWithBodies(ctx, namespace, "app", nil, func(obj *storedObject) error {
		calls++

		require.Equal(t, annotations, obj.Annotations)

		return nil
	}))
	require.Equal(t, 1, calls)

	require.NoError(t, b.updateLabels(ctx, namespace, obj.Key, map[string]string{"team": "new"}))
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, annotations, stored.Annotations)

	delete(annotations, "removed")
	obj.Annotations = maps.Clone(annotations)
	obj.Labels[storageLabelModifiedAt] = "200"
	require.NoError(t, b.update(ctx, obj))
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, annotations, stored.Annotations)

	var annotationRows int
	require.NoError(t, b.db.GetContext(ctx, &annotationRows, `SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1 AND releaseModifiedAt = 200`, namespace))
	require.Equal(t, len(annotations), annotationRows)

	legacy := newTestRelease(namespace, "app", 1, common.StatusDeployed)
	legacy.Info.Annotations = map[string]string{"foreign": "legacy"}
	legacyBody := encodeHelmRelease(t, legacy)
	_, err = b.db.ExecContext(ctx, "UPDATE releases_v1 SET body = $1 WHERE namespace = $2 AND key = $3", string(legacyBody), namespace, obj.Key)
	require.NoError(t, err)
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, annotations, stored.Annotations, "a same-second foreign write is intentionally undetected")

	_, err = b.db.ExecContext(ctx, "UPDATE releases_v1 SET modifiedAt = modifiedAt + 1 WHERE namespace = $1 AND key = $2", namespace, obj.Key)
	require.NoError(t, err)
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Nil(t, stored.Annotations)
	decoded, err := decodeStoredObject(stored)
	require.NoError(t, err)
	require.Equal(t, legacy.Info.Annotations, decoded.Info.Annotations)

	metadata, err = b.listMetadata(ctx, namespace, "app", true)
	require.NoError(t, err)
	require.Nil(t, metadata[0].Annotations)

	for _, withBodies := range []bool{false, true} {
		require.NoError(t, b.scanLatestCandidates(ctx, namespace, nil, withBodies, func(obj *storedObject) error {
			require.Nil(t, obj.Annotations)

			return nil
		}))
	}

	require.NoError(t, b.listWithBodies(ctx, namespace, "app", nil, func(obj *storedObject) error {
		require.Nil(t, obj.Annotations)

		return nil
	}))
	require.NoError(t, b.db.GetContext(ctx, &annotationRows, `SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1`, namespace))
	require.Equal(t, len(annotations), annotationRows, "reads do not delete stale metadata")

	obj.Annotations = map[string]string{"nelm.werf.io/released-by": "nelm"}
	require.NoError(t, b.update(ctx, obj))
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, obj.Annotations, stored.Annotations)
	obj.Annotations = nil
	require.NoError(t, b.update(ctx, obj))
	stored, err = b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Nil(t, stored.Annotations, "manual legacy objects are not marked automatically")

	obj.Annotations = maps.Clone(annotations)
	require.NoError(t, b.update(ctx, obj))
	require.NoError(t, b.delete(ctx, namespace, obj.Key))
	require.NoError(t, b.db.GetContext(ctx, &annotationRows, `SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1`, namespace))
	require.Zero(t, annotationRows)
}

func TestSQLStorageBackend_ExplicitAnnotationPoliciesAllowStatusTransitionPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	b.db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-transition-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	storage := newReleaseStorage(namespace, b, 0)
	rls := newTestRelease(namespace, "app", 1, common.StatusPendingInstall)
	rls.Info.Annotations = map[string]string{"team": "platform"}
	accessor, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, storage.Create(ctx, accessor))

	role := "nelm_transition_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() {
		_, err := b.db.ExecContext(ctx, "RESET ROLE")
		require.NoError(t, err)
		require.NoError(t, storage.Delete(ctx, "app", 1))

		for _, query := range []string{
			"DROP POLICY IF EXISTS " + pq.QuoteIdentifier(role+"_read") + " ON releases_v1",
			"DROP POLICY IF EXISTS " + pq.QuoteIdentifier(role+"_write") + " ON releases_v1",
			"DROP POLICY IF EXISTS " + pq.QuoteIdentifier(role) + " ON nelm_release_annotations_v1",
			"DROP ROLE IF EXISTS " + pq.QuoteIdentifier(role),
		} {
			_, err = b.db.ExecContext(ctx, query)
			require.NoError(t, err)
		}
	})

	for _, query := range []string{
		"CREATE ROLE " + pq.QuoteIdentifier(role),
		"CREATE POLICY " + pq.QuoteIdentifier(role+"_read") + " ON releases_v1 FOR SELECT TO " + pq.QuoteIdentifier(role) + " USING (namespace = " + pq.QuoteLiteral(namespace) + ")",
		"CREATE POLICY " + pq.QuoteIdentifier(role+"_write") + " ON releases_v1 FOR UPDATE TO " + pq.QuoteIdentifier(role) + " USING (namespace = " + pq.QuoteLiteral(namespace) + " AND status = 'pending-install') WITH CHECK (namespace = " + pq.QuoteLiteral(namespace) + " AND status IN ('pending-install','deployed'))",
		"CREATE POLICY " + pq.QuoteIdentifier(role) + " ON nelm_release_annotations_v1 TO " + pq.QuoteIdentifier(role) + " USING (releaseNamespace = " + pq.QuoteLiteral(namespace) + ") WITH CHECK (releaseNamespace = " + pq.QuoteLiteral(namespace) + ")",
		"SET ROLE " + pq.QuoteIdentifier(role),
	} {
		_, err = b.db.ExecContext(ctx, query)
		require.NoError(t, err)
	}

	loaded, err := storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	loaded.SetStatus(common.StatusDeployed)
	require.NoError(t, storage.Update(ctx, loaded))
	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, "deployed", loaded.Status())
	require.Equal(t, "platform", loaded.Annotations()["team"])

	loaded.SetStatus(common.StatusFailed)
	require.Error(t, storage.Update(ctx, loaded))
	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, "deployed", loaded.Status())
	require.Equal(t, "platform", loaded.Annotations()["team"])
}

func TestSQLStorageBackend_ExplicitPoliciesOpenForNonDDLRolePostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-noddl-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	role := "nelm_noddl_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = b.db.ExecContext(ctx, "CREATE ROLE "+pq.QuoteIdentifier(role)+" LOGIN PASSWORD 'nelm-test'")
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, table := range []string{sqlReleaseTable, sqlCustomLabelsTable, sqlAnnotationsTable} {
			_, err := b.db.ExecContext(ctx, "DROP POLICY IF EXISTS "+pq.QuoteIdentifier(role)+" ON "+table)
			require.NoError(t, err)
		}

		_, err := b.db.ExecContext(ctx, "REVOKE SELECT ON gorp_migrations FROM "+pq.QuoteIdentifier(role))
		require.NoError(t, err)
		_, err = b.db.ExecContext(ctx, "DROP ROLE IF EXISTS "+pq.QuoteIdentifier(role))
		require.NoError(t, err)
	})

	for _, table := range []string{sqlReleaseTable, sqlCustomLabelsTable, sqlAnnotationsTable} {
		column := "releaseNamespace"
		if table == sqlReleaseTable {
			column = "namespace"
		}

		_, err = b.db.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON "+table+" TO "+pq.QuoteIdentifier(role)+" USING ("+column+" = "+pq.QuoteLiteral(namespace)+") WITH CHECK ("+column+" = "+pq.QuoteLiteral(namespace)+")")
		require.NoError(t, err)
	}

	var canReadHelmMigrations bool
	require.NoError(t, b.db.GetContext(ctx, &canReadHelmMigrations, "SELECT has_table_privilege($1, 'gorp_migrations', 'SELECT')", role))
	require.False(t, canReadHelmMigrations)

	_, err = b.db.ExecContext(ctx, "GRANT SELECT ON gorp_migrations TO "+pq.QuoteIdentifier(role))
	require.NoError(t, err)

	roleConnection := connection + " user=" + role + " password=nelm-test"
	if strings.HasPrefix(connection, "postgres://") || strings.HasPrefix(connection, "postgresql://") {
		parsed, err := url.Parse(connection)
		require.NoError(t, err)

		query := parsed.Query()
		query.Del("user")
		query.Del("password")
		parsed.RawQuery = query.Encode()
		parsed.User = url.UserPassword(role, "nelm-test")
		roleConnection = parsed.String()
	}

	limited, err := openSQLStorageBackend(ctx, roleConnection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, limited.db.Close()) })

	var sessionUser string
	require.NoError(t, limited.db.GetContext(ctx, &sessionUser, "SELECT session_user"))
	require.Equal(t, role, sessionUser)

	storage := newReleaseStorage(namespace, limited, 0)
	rls := newTestRelease(namespace, "app", 1, common.StatusPendingInstall)
	rls.Info.Annotations = map[string]string{"team": "platform"}
	rls.Labels = map[string]string{"team": "platform"}
	accessor, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, storage.Create(ctx, accessor))
	t.Cleanup(func() { require.NoError(t, storage.Delete(ctx, "app", 1)) })

	loaded, err := storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, "platform", loaded.Annotations()["team"])
	loaded.SetStatus(common.StatusDeployed)
	require.NoError(t, storage.Update(ctx, loaded))
	require.NoError(t, storage.UpdateLabels(ctx, "app", 1, map[string]string{"team": "changed"}))
	loaded, err = storage.GetRelease(ctx, "app", 1)
	require.NoError(t, err)
	require.Equal(t, "deployed", loaded.Status())
	require.Equal(t, "platform", loaded.Annotations()["team"])
	require.Equal(t, "changed", loaded.Labels()["team"])
}

func TestSQLStorageBackend_HiddenCreatedReleaseRollsBackAnnotationsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	b.db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-insert-only-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	role := "nelm_insert_only_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = b.db.ExecContext(ctx, "CREATE ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := b.db.ExecContext(ctx, "RESET ROLE")
		require.NoError(t, err)
		_, err = b.db.ExecContext(ctx, "DELETE FROM releases_v1 WHERE namespace = $1", namespace)
		require.NoError(t, err)
		_, err = b.db.ExecContext(ctx, "DROP POLICY IF EXISTS "+pq.QuoteIdentifier(role)+" ON releases_v1")
		require.NoError(t, err)
		_, err = b.db.ExecContext(ctx, "DROP ROLE "+pq.QuoteIdentifier(role))
		require.NoError(t, err)
	})

	_, err = b.db.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON releases_v1 FOR INSERT TO "+pq.QuoteIdentifier(role)+" WITH CHECK (namespace = "+pq.QuoteLiteral(namespace)+")")
	require.NoError(t, err)
	_, err = b.db.ExecContext(ctx, "SET ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)

	rls := newTestRelease(namespace, "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "platform"}
	obj := newTestStoredObject(t, rls)
	require.Error(t, b.create(ctx, obj), "a hidden release must not commit without its metadata annotations")
	_, err = b.db.ExecContext(ctx, "RESET ROLE")
	require.NoError(t, err)

	var count int
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM releases_v1 WHERE namespace = $1", namespace))
	require.Zero(t, count)
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1", namespace))
	require.Zero(t, count)
}

func TestSQLStorageBackend_LatestIgnoresRowsOutsideTheirStorageKeyPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-key-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	original := newTestStoredObject(t, newTestRelease(namespace, "app", 1, common.StatusDeployed))
	backup := newTestStoredObject(t, newTestRelease(namespace, "app", 2, common.StatusDeployed))
	backup.Key = "app-backup"

	for _, obj := range []*storedObject{original, backup} {
		require.NoError(t, b.create(ctx, obj))
		t.Cleanup(func() { require.NoError(t, b.delete(ctx, obj.Namespace, obj.Key)) })
	}

	for _, withBodies := range []bool{true, false} {
		var keys []string
		require.NoError(t, b.scanLatestCandidates(ctx, namespace, labels.Everything(), withBodies, func(obj *storedObject) error {
			keys = append(keys, obj.Key)

			return nil
		}))
		require.Equal(t, []string{original.Key}, keys, "bodies %t", withBodies)
	}
}

func TestSQLStorageBackend_LongReleaseAnnotationKeyPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-long-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	storage := newReleaseStorage(namespace, b, 0)
	rls := newTestRelease(namespace, strings.Repeat("a", 53), 1, common.StatusDeployed)
	accessor, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, storage.Create(ctx, accessor))
	t.Cleanup(func() { require.NoError(t, storage.Delete(ctx, rls.Name, 1)) })

	loaded, err := storage.GetRelease(ctx, rls.Name, 1)
	require.NoError(t, err)
	require.Equal(t, storageAnnotationReleasedByValue, loaded.Annotations()[storageAnnotationReleasedBy])

	loaded.SetStatus(common.StatusSuperseded)
	require.NoError(t, storage.Update(ctx, loaded))

	legacyRelease := newTestRelease(namespace, rls.Name, 2, common.StatusDeployed)
	legacyRelease.Info.Annotations = map[string]string{"team": "legacy"}
	legacy, err := newStoredObject(namespace, legacyRelease, storageLabelCreatedAt)
	require.NoError(t, err)

	legacy.Annotations = nil
	legacy.Body = encodeHelmRelease(t, legacyRelease)
	require.NoError(t, b.create(ctx, legacy))
	t.Cleanup(func() { require.NoError(t, storage.Delete(ctx, rls.Name, 2)) })

	loaded, err = storage.GetRelease(ctx, rls.Name, 2)
	require.NoError(t, err)
	require.Equal(t, legacyRelease.Info.Annotations, loaded.Annotations())
	require.NoError(t, storage.Update(ctx, loaded))
	loaded, err = storage.GetRelease(ctx, rls.Name, 2)
	require.NoError(t, err)
	require.Equal(t, "legacy", loaded.Annotations()["team"])
	require.Equal(t, storageAnnotationReleasedByValue, loaded.Annotations()[storageAnnotationReleasedBy])
}

func TestSQLStorageBackend_ReadOnlyReleaseRoleCannotChangeAnnotationsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-readonly-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	obj := newTestStoredObject(t, newTestRelease(namespace, "app", 1, common.StatusDeployed))
	require.NoError(t, b.create(ctx, obj))
	t.Cleanup(func() { require.NoError(t, b.delete(ctx, namespace, obj.Key)) })

	tx, err := b.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { rollbackSQLTransaction(ctx, tx) })

	role := "nelm_readonly_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = tx.ExecContext(ctx, "CREATE ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON releases_v1 FOR SELECT TO "+pq.QuoteIdentifier(role)+" USING (namespace = "+pq.QuoteLiteral(namespace)+")")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role+"_write")+" ON releases_v1 FOR UPDATE TO "+pq.QuoteIdentifier(role)+" USING (namespace = "+pq.QuoteLiteral(namespace)+") WITH CHECK (false)")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "CREATE POLICY "+pq.QuoteIdentifier(role)+" ON nelm_release_annotations_v1 FOR SELECT TO "+pq.QuoteIdentifier(role)+" USING (releaseNamespace = "+pq.QuoteLiteral(namespace)+")")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+pq.QuoteIdentifier(role))
	require.NoError(t, err)

	var count int
	require.NoError(t, tx.GetContext(ctx, &count, "SELECT count(*) FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1", namespace))
	require.Equal(t, 1, count)

	_, err = tx.ExecContext(ctx, "SAVEPOINT denied_release_update")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE releases_v1 SET status = status WHERE namespace = $1", namespace)
	require.ErrorContains(t, err, "row-level security policy")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT denied_release_update")
	require.NoError(t, err)

	result, err := tx.ExecContext(ctx, "UPDATE nelm_release_annotations_v1 SET value = 'spoofed' WHERE releaseNamespace = $1", namespace)
	require.NoError(t, err)
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	require.Zero(t, affected)

	result, err = tx.ExecContext(ctx, "DELETE FROM nelm_release_annotations_v1 WHERE releaseNamespace = $1", namespace)
	require.NoError(t, err)
	affected, err = result.RowsAffected()
	require.NoError(t, err)
	require.Zero(t, affected)

	_, err = tx.ExecContext(ctx, "INSERT INTO nelm_release_annotations_v1 (releaseKey, releaseNamespace, key, value, releaseModifiedAt) VALUES ($1, $2, 'spoof', 'value', 0)", obj.Key, namespace)
	require.ErrorContains(t, err, "row-level security policy")
	require.NoError(t, tx.Rollback())
}

func TestSQLStorageBackend_SelectorsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	prefix := "nelm-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	specs := []struct {
		ns, name string
		version  int
		custom   map[string]string
	}{
		{ns: prefix + "-one", name: "same", version: 1, custom: map[string]string{"packageChecksum": "", "team": "old", "size": "0000000000000000000000000000000000000000000000000000000000010"}},
		{ns: prefix + "-one", name: "same", version: 2, custom: map[string]string{"packageChecksum": "new", "team": "new", "size": "9223372036854775808"}},
		{ns: prefix + "-two", name: "same", version: 3, custom: map[string]string{"team": "old", "size": "9223372036854775807"}},
		{ns: prefix + "-two", name: "other", version: 1, custom: map[string]string{"size": "text"}},
	}

	objects := []*storedObject{}
	for _, spec := range specs {
		r := newTestRelease(spec.ns, spec.name, spec.version, common.StatusDeployed)
		r.Labels = spec.custom
		obj := newTestStoredObject(t, r)
		require.NoError(t, b.create(ctx, obj))
		t.Cleanup(func() { require.NoError(t, b.delete(ctx, obj.Namespace, obj.Key)) })

		objects = append(objects, obj)
	}

	for _, raw := range []string{"", "packageChecksum", "!packageChecksum", "team=old", "team!=new", "team in (old,new)", "team notin (new)", "version in (1,3)", "status=deployed", "modifiedAt", "!modifiedAt", "size>9", "size<11", "size>9223372036854775806", "owner!=helm", "owner=foreign"} {
		t.Run(raw, func(t *testing.T) {
			sel, err := labels.Parse(raw)
			require.NoError(t, err)

			expected := map[string]int{}
			for _, obj := range objects {
				if !sel.Matches(labels.Set(obj.Labels)) {
					continue
				}

				rev, ok, err := revisionFromStoredObject(obj)
				require.NoError(t, err)
				require.True(t, ok)

				id := rev.Namespace + "/" + rev.Name
				if rev.Version > expected[id] {
					expected[id] = rev.Version
				}
			}

			for _, withBodies := range []bool{true, false} {
				actual := map[string]int{}
				err = b.scanLatestCandidates(ctx, "", sel, withBodies, func(obj *storedObject) error {
					if obj.Namespace != prefix+"-one" && obj.Namespace != prefix+"-two" {
						return nil
					}

					rev, ok, err := revisionFromStoredObject(obj)
					require.NoError(t, err)
					require.True(t, ok)
					require.Equal(t, withBodies, len(obj.Body) > 0)

					actual[rev.Namespace+"/"+rev.Name] = rev.Version
					for _, original := range objects {
						if original.Namespace == obj.Namespace && original.Key == obj.Key {
							require.Equal(t, original.Labels, obj.Labels)
						}
					}

					return nil
				})
				require.NoError(t, err)
				require.Equal(t, expected, actual, "selector %q, bodies %t", raw, withBodies)
			}
		})
	}
}

func TestSQLStorageBackend_UpdateLabelsWaitsForReleaseRowLockPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-lock-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	rls := newTestRelease(namespace, "myrel", 1, common.StatusDeployed)
	rls.Labels = map[string]string{"team": "old"}
	obj := newTestStoredObject(t, rls)
	require.NoError(t, b.create(ctx, obj))
	t.Cleanup(func() { require.NoError(t, b.delete(ctx, obj.Namespace, obj.Key)) })

	tx, err := b.db.BeginTxx(ctx, nil)
	require.NoError(t, err)

	// Runs before the delete cleanup, releasing the row lock on failure.
	t.Cleanup(func() { rollbackSQLTransaction(ctx, tx) })

	_, err = tx.ExecContext(ctx, "UPDATE releases_v1 SET status = status WHERE key = $1 AND namespace = $2", obj.Key, namespace)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "DELETE FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2", obj.Key, namespace)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- b.updateLabels(ctx, namespace, obj.Key, map[string]string{"leftover": "yes"})
	}()

	select {
	case err := <-done:
		t.Fatalf("label update did not wait for the release row lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO custom_labels_v1 (releaseKey, releaseNamespace, key, value) VALUES ($1, $2, 'team', 'new')", obj.Key, namespace)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, <-done)

	stored, err := b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, "new", stored.Labels["team"])
	require.Equal(t, "yes", stored.Labels["leftover"])

	var teamRows int
	require.NoError(t, b.db.GetContext(ctx, &teamRows, "SELECT count(*) FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2 AND key = 'team'", obj.Key, namespace))
	require.Equal(t, 1, teamRows)
}

func TestSQLStorageBackend_UpdateReplacesCustomLabelsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := openSQLStorageBackend(ctx, connection)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.db.Close()) })

	namespace := "nelm-test-update-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	rls := newTestRelease(namespace, "myrel", 1, common.StatusDeployed)
	rls.Labels = map[string]string{"team": "old", "removed": "yes"}
	obj := newTestStoredObject(t, rls)
	require.NoError(t, b.create(ctx, obj))
	t.Cleanup(func() { require.NoError(t, b.delete(ctx, obj.Namespace, obj.Key)) })

	rls.Labels = map[string]string{"team": "new"}
	updated, err := newStoredObject(namespace, rls, storageLabelModifiedAt)
	require.NoError(t, err)
	require.NoError(t, b.update(ctx, updated))

	stored, err := b.get(ctx, namespace, obj.Key)
	require.NoError(t, err)
	require.Equal(t, "new", stored.Labels["team"])
	require.NotContains(t, stored.Labels, "removed")

	var count int
	require.NoError(t, b.db.GetContext(ctx, &count, "SELECT count(*) FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2", obj.Key, namespace))
	require.Equal(t, 1, count)

	missing, err := newStoredObject(namespace, newTestRelease(namespace, "missing", 1, common.StatusDeployed), storageLabelModifiedAt)
	require.NoError(t, err)
	require.ErrorIs(t, b.update(ctx, missing), ErrReleaseNotFound)
}
