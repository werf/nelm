package release

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestEnsureSQLStorageSchema_SkipsMigrationsWhenAllApplied(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectQuery(`SELECT * FROM "gorp_migrations" ORDER BY "id" ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "applied_at"}).
			AddRow("custom_labels", time.Now()).
			AddRow("init", time.Now()))

	require.NoError(t, ensureSQLStorageSchema(context.Background(), backend.db))
}

func TestSQLStorageBackend_CreateExisting(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	obj := newTestStoredObject(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusPendingInstall))

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO releases_v1 (key,type,body,name,namespace,version,status,owner,createdAt) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`).
		WillReturnError(&pq.Error{Code: pqerror.UniqueViolation})
	mock.ExpectRollback()

	require.ErrorIs(t, backend.create(context.Background(), obj), ErrReleaseExists)
}

func TestSQLStorageBackend_CreateInsertsReleaseAndCustomLabels(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	obj := newTestStoredObject(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusPendingInstall))
	obj.Labels["custom"] = "value"

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO releases_v1 (key,type,body,name,namespace,version,status,owner,createdAt) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`).
		WithArgs(obj.Key, "helm.sh/release.v1", string(obj.Body), "myrel", testNamespace, 1, "pending-install", "helm", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO custom_labels_v1 (releaseKey,releaseNamespace,key,value) VALUES ($1,$2,$3,$4)`).
		WithArgs(obj.Key, testNamespace, "custom", "value").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	require.NoError(t, backend.create(context.Background(), obj))
}

func TestSQLStorageBackend_DeleteMissing(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	require.ErrorIs(t, backend.delete(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v1"), ErrReleaseNotFound)
}

func TestSQLStorageBackend_DeleteRemovesCustomLabels(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	require.NoError(t, backend.delete(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v1"))
}

func TestSQLStorageBackend_ForEachReleaseReadsLabelsBeforeBodies(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	body := encodeHelmRelease(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusSuperseded))

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt FROM releases_v1 WHERE owner = $1 AND namespace = $2 AND name = $3`).
		WithArgs("helm", testNamespace, "myrel").
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat"}).
			AddRow("sh.helm.release.v1.myrel.v1", testNamespace, "myrel", 1, "superseded", "helm", 100, 0).
			AddRow("sh.helm.release.v1.myrel.v2", testNamespace, "myrel", 2, "deployed", "helm", 200, 0))
	mock.ExpectQuery(`SELECT releaseKey, key, value FROM custom_labels_v1 WHERE releaseKey IN ($1,$2) AND releaseNamespace = $3`).
		WithArgs("sh.helm.release.v1.myrel.v1", "sh.helm.release.v1.myrel.v2", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"releasekey", "key", "value"}).AddRow("sh.helm.release.v1.myrel.v1", "custom", "value"))
	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt, body FROM releases_v1 WHERE owner = $1 AND namespace = $2 AND name = $3 ORDER BY version ASC`).
		WithArgs("helm", testNamespace, "myrel").
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat", "body"}).
			AddRow("sh.helm.release.v1.myrel.v1", testNamespace, "myrel", 1, "superseded", "helm", 100, 0, string(body)).
			AddRow("sh.helm.release.v1.myrel.v2", testNamespace, "myrel", 2, "deployed", "helm", 200, 0, "corrupt"))

	type entry struct {
		custom  string
		err     error
		version int
	}

	var entries []entry
	require.NoError(t, newReleaseStorage(testNamespace, backend, 0).ForEachRelease(context.Background(), "myrel", func(revision Revision, rel helmrel.Accessor, err error) error {
		e := entry{err: err, version: revision.Version}
		if rel != nil {
			e.custom = rel.Labels()["custom"]
		}

		entries = append(entries, e)

		return nil
	}))

	require.Len(t, entries, 2)
	assert.Equal(t, entry{custom: "value", version: 1}, entries[0])
	assert.Equal(t, 2, entries[1].version)
	require.ErrorIs(t, entries[1].err, ErrReleaseUndecodable)
}

func TestSQLStorageBackend_GetMergesCustomLabels(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	body := encodeHelmRelease(t, newTestReleaseWithStatus("myrel", 2, helmreleasecommon.StatusDeployed))

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt, body FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v2", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat", "body"}).
			AddRow("sh.helm.release.v1.myrel.v2", testNamespace, "myrel", 2, "deployed", "helm", 100, 0, string(body)))
	mock.ExpectQuery(`SELECT key, value FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v2", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("custom", "value").AddRow("status", "bogus"))

	obj, err := backend.get(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v2")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"name":      "myrel",
		"owner":     "helm",
		"status":    "deployed",
		"version":   "2",
		"createdAt": "100",
		"custom":    "value",
	}, obj.Labels)
	assert.Equal(t, body, obj.Body)
}

func TestSQLStorageBackend_GetMissing(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt, body FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v2", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"key"}))

	_, err := backend.get(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v2")
	require.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestSQLStorageBackend_ListMetadataReadsNoBodies(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt FROM releases_v1 WHERE owner = $1`).
		WithArgs("helm").
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat"}).
			AddRow("sh.helm.release.v1.a.v1", "ns-1", "a", 1, "superseded", "helm", 100, 200).
			AddRow("sh.helm.release.v1.a.v2", "ns-2", "a", 2, "deployed", "helm", 300, 0))

	storage := newReleaseStorage("", backend, 0)

	revisions, err := storage.LatestRevisions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Revision{
		{Name: "a", Namespace: "ns-1", Status: "superseded", Version: 1},
		{Name: "a", Namespace: "ns-2", Status: "deployed", Version: 2},
	}, revisions)
}

func TestSQLStorageBackend_RevisionsOfRelease(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt FROM releases_v1 WHERE owner = $1 AND namespace = $2 AND name = $3`).
		WithArgs("helm", testNamespace, "a").
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat"}).
			AddRow("sh.helm.release.v1.a.v10", testNamespace, "a", 10, "deployed", "helm", 100, 0).
			AddRow("sh.helm.release.v1.a.v9", testNamespace, "a", 9, "superseded", "helm", 100, 0))

	revisions, err := newReleaseStorage(testNamespace, backend, 0).Revisions(context.Background(), "a")
	require.NoError(t, err)
	assert.Equal(t, []Revision{
		{Name: "a", Namespace: testNamespace, Status: "superseded", Version: 9},
		{Name: "a", Namespace: testNamespace, Status: "deployed", Version: 10},
	}, revisions)
}

func TestSQLStorageBackend_UpdateLabelsOfMissingRelease(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT key FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"key"}))
	mock.ExpectRollback()

	require.ErrorIs(t, backend.updateLabels(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v1", map[string]string{"custom": "new"}), ErrReleaseNotFound)
}

func TestSQLStorageBackend_UpdateLabelsReplacesValues(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT key FROM releases_v1 WHERE key = $1 AND namespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("sh.helm.release.v1.myrel.v1"))
	mock.ExpectExec(`DELETE FROM custom_labels_v1 WHERE key = $1 AND releaseKey = $2 AND releaseNamespace = $3`).
		WithArgs("custom", "sh.helm.release.v1.myrel.v1", testNamespace).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO custom_labels_v1 (releaseKey,releaseNamespace,key,value) VALUES ($1,$2,$3,$4)`).
		WithArgs("sh.helm.release.v1.myrel.v1", testNamespace, "custom", "new").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	require.NoError(t, backend.updateLabels(context.Background(), testNamespace, "sh.helm.release.v1.myrel.v1", map[string]string{"custom": "new"}))
}

func TestSQLStorageBackend_UpdateMissing(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	obj, err := newStoredObject(testNamespace, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed), storageLabelModifiedAt)
	require.NoError(t, err)

	mock.ExpectExec(`UPDATE releases_v1 SET body = $1, name = $2, version = $3, status = $4, owner = $5, modifiedAt = $6 WHERE key = $7 AND namespace = $8`).
		WithArgs(string(obj.Body), "myrel", 1, "deployed", "helm", sqlmock.AnyArg(), obj.Key, testNamespace).
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.ErrorIs(t, backend.update(context.Background(), obj), ErrReleaseNotFound)
}
