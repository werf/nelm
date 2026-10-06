package release

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

const (
	testSQLLatestColumns = "key, namespace, name, version, status, owner, createdAt, modifiedAt, body"
	testSQLLatestOrder   = " ORDER BY namespace, name, version DESC) AS releases_v1"
	testSQLLatestSelect  = "SELECT releases_v1.*, COALESCE((SELECT json_object_agg(c.key, c.value ORDER BY c.ctid)::text FROM custom_labels_v1 c WHERE c.releaseKey = releases_v1.key AND c.releaseNamespace = releases_v1.namespace), '{}') AS custom_labels FROM (SELECT DISTINCT ON (namespace, name) " + testSQLLatestColumns + " FROM releases_v1 WHERE owner = $1"
)

func TestEnsureSQLStorageSchema_SkipsMigrationsWhenAllApplied(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	mock.ExpectQuery(`SELECT * FROM "gorp_migrations" ORDER BY "id" ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "applied_at"}).
			AddRow("custom_labels", time.Now()).
			AddRow("init", time.Now()))

	require.NoError(t, ensureSQLStorageSchema(context.Background(), backend.db))
}

func TestSQLLabelRequirementUnsupportedOperator(t *testing.T) {
	_, err := sqlLabelRequirement(labels.Requirement{})
	require.ErrorContains(t, err, "unsupported operator")
	require.ErrorContains(t, err, "use equality, set, existence, or integer comparisons")
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
	}, ForEachReleaseOptions{}))

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

func TestSQLStorageBackend_ListLatestCancelledDuringCallback(t *testing.T) {
	backend, mock := newTestSQLBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rows := sqlmock.NewRows([]string{"key", "custom_labels"}).AddRow("a", "{}").AddRow("b", "{}")
	mock.ExpectQuery(testSQLLatestSelect + testSQLLatestOrder).WithArgs("helm").WillReturnRows(rows).RowsWillBeClosed()
	calls := 0
	err := backend.listLatestWithBodies(ctx, "", nil, func(*storedObject) error {
		calls++
		cancel()

		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func TestSQLStorageBackend_ListLatestCloseError(t *testing.T) {
	backend, mock := newTestSQLBackend(t)
	sentinel := errors.New("close failed")
	mock.ExpectQuery(testSQLLatestSelect + testSQLLatestOrder).WithArgs("helm").WillReturnRows(sqlmock.NewRows([]string{"key"}).CloseError(sentinel)).RowsWillBeClosed()
	require.ErrorIs(t, backend.listLatestWithBodies(context.Background(), "", nil, func(*storedObject) error { return nil }), sentinel)
}

func TestSQLStorageBackend_ListLatestErrors(t *testing.T) {
	sentinel := errors.New("failed")
	for _, kind := range []string{"query", "scan", "labels", "iterate", "callback", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			backend, mock := newTestSQLBackend(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expectation := mock.ExpectQuery(testSQLLatestSelect + testSQLLatestOrder).WithArgs("helm")
			rows := sqlmock.NewRows([]string{"key", "namespace", "version", "custom_labels"}).AddRow("a", "ns", 1, "{}")
			switch kind {
			case "query":
				expectation.WillReturnError(sentinel)
			case "scan":
				expectation.WillReturnRows(sqlmock.NewRows([]string{"unknown"}).AddRow("bad")).RowsWillBeClosed()
			case "labels":
				expectation.WillReturnRows(sqlmock.NewRows([]string{"custom_labels"}).AddRow("bad")).RowsWillBeClosed()
			case "iterate":
				expectation.WillReturnRows(rows.RowError(0, sentinel)).RowsWillBeClosed()
			case "cancel":
				expectation.WillDelayFor(time.Second).WillReturnRows(rows)
				time.AfterFunc(10*time.Millisecond, cancel)
			default:
				expectation.WillReturnRows(rows.AddRow("b", "ns", 1, "{}")).RowsWillBeClosed()
			}
			err := backend.listLatestWithBodies(ctx, "", nil, func(*storedObject) error { return sentinel })
			require.Error(t, err)
			if kind == "callback" || kind == "iterate" || kind == "query" {
				require.ErrorIs(t, err, sentinel)
			}
			if kind == "cancel" {
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			}
		})
	}
}

func TestSQLStorageBackend_ListLatestIntegerSelectors(t *testing.T) {
	for _, tt := range []struct {
		selector, column, operator string
		threshold                  int64
		custom                     bool
	}{
		{selector: "packageChecksum>10", column: "value", operator: ">", threshold: 10, custom: true},
		{selector: "packageChecksum<10", column: "value", operator: "<", threshold: 10, custom: true},
		{selector: "version>10", column: "version::text", operator: ">", threshold: 10, custom: false},
		{selector: "createdAt<100", column: "createdAt::text", operator: "<", threshold: 100, custom: false},
		{selector: "modifiedAt>0", column: "NULLIF(modifiedAt, 0)::text", operator: ">", threshold: 0, custom: false},
		{selector: "name>9223372036854775807", column: "name", operator: ">", threshold: 9223372036854775807, custom: false},
	} {
		t.Run(tt.selector, func(t *testing.T) {
			backend, mock := newTestSQLBackend(t)
			selector, err := labels.Parse(tt.selector)
			require.NoError(t, err)
			requirements, _ := selector.Requirements()
			number := "(CASE WHEN " + tt.column + " ~ '^[+-]?[0-9]+$' AND length(regexp_replace(" + tt.column + ", '^[+-]?0*', '')) <= 19 THEN (" + tt.column + ")::numeric END)"
			placeholder := "$2"
			args := []driver.Value{"helm", tt.threshold}
			if tt.custom {
				placeholder = "$3"
				args = []driver.Value{"helm", "packageChecksum", tt.threshold}
			}
			predicate := number + " BETWEEN -9223372036854775808 AND 9223372036854775807 AND " + number + " " + tt.operator + " " + placeholder
			if tt.custom {
				predicate = "EXISTS (SELECT 1 FROM (SELECT value FROM custom_labels_v1 WHERE releaseKey = releases_v1.key AND releaseNamespace = releases_v1.namespace AND key = $2 ORDER BY ctid DESC LIMIT 1) AS label WHERE " + predicate + ")"
			}
			mock.ExpectQuery(testSQLLatestSelect + " AND " + predicate + testSQLLatestOrder).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"key"})).RowsWillBeClosed()
			require.NoError(t, backend.listLatestWithBodies(context.Background(), "", selector, func(*storedObject) error {
				t.Fatal("unexpected callback")

				return nil
			}))
			for _, invalid := range []string{"text", "", "1.5", " 11", "9223372036854775808", "-9223372036854775809", "18446744073709551616"} {
				assert.False(t, selector.Matches(labels.Set{requirements[0].Key(): invalid}))
			}
		})
	}
}

func TestSQLStorageBackend_ListLatestMatchingRevision(t *testing.T) {
	backend, mock := newTestSQLBackend(t)
	selector, err := labels.Parse("packageChecksum,status=superseded")
	require.NoError(t, err)
	mock.ExpectQuery(testSQLLatestSelect+" AND namespace = $2 AND EXISTS (SELECT 1 FROM (SELECT value FROM custom_labels_v1 WHERE releaseKey = releases_v1.key AND releaseNamespace = releases_v1.namespace AND key = $3 ORDER BY ctid DESC LIMIT 1) AS label WHERE value IS NOT NULL) AND status IN ($4)"+testSQLLatestOrder).
		WithArgs("helm", testNamespace, "packageChecksum", "superseded").
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat", "body", "custom_labels"}).
			AddRow("sh.helm.release.v1.myrel.v1", testNamespace, "myrel", 1, "superseded", "helm", 100, 0, "selected-body", `{"packageChecksum":"old","packageChecksum":"matching"}`)).RowsWillBeClosed()
	calls := 0
	require.NoError(t, backend.listLatestWithBodies(context.Background(), testNamespace, selector, func(obj *storedObject) error {
		calls++
		assert.Equal(t, "1", obj.Labels["version"])
		assert.Equal(t, "matching", obj.Labels["packageChecksum"])
		assert.Equal(t, []byte("selected-body"), obj.Body)
		assert.True(t, selector.Matches(labels.Set(obj.Labels)))

		return nil
	}))
	assert.Equal(t, 1, calls)
}

func TestSQLStorageBackend_ListLatestNothing(t *testing.T) {
	backend, mock := newTestSQLBackend(t)
	mock.ExpectQuery(testSQLLatestSelect + " AND FALSE" + testSQLLatestOrder).WithArgs("helm").WillReturnRows(sqlmock.NewRows([]string{"key"}))
	require.NoError(t, backend.listLatestWithBodies(context.Background(), "", labels.Nothing(), func(*storedObject) error {
		t.Fatal("unexpected callback")

		return nil
	}))
}

func TestSQLStorageBackend_ListLatestSelector(t *testing.T) {
	custom := "SELECT value FROM custom_labels_v1 WHERE releaseKey = releases_v1.key AND releaseNamespace = releases_v1.namespace AND key = $3 ORDER BY ctid DESC LIMIT 1"
	tests := []struct {
		selector, predicate string
		args                []driver.Value
	}{
		{selector: "packageChecksum", predicate: "EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IS NOT NULL)", args: []driver.Value{"packageChecksum"}},
		{selector: "!packageChecksum", predicate: "NOT EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IS NOT NULL)", args: []driver.Value{"packageChecksum"}},
		{selector: "packageChecksum=abc", predicate: "EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IN ($4))", args: []driver.Value{"packageChecksum", "abc"}},
		{selector: "packageChecksum==abc", predicate: "EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IN ($4))", args: []driver.Value{"packageChecksum", "abc"}},
		{selector: "packageChecksum!=abc", predicate: "NOT EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IN ($4))", args: []driver.Value{"packageChecksum", "abc"}},
		{selector: "packageChecksum in (abc,def)", predicate: "EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IN ($4,$5))", args: []driver.Value{"packageChecksum", "abc", "def"}},
		{selector: "packageChecksum notin (abc,def)", predicate: "NOT EXISTS (SELECT 1 FROM (" + custom + ") AS label WHERE value IN ($4,$5))", args: []driver.Value{"packageChecksum", "abc", "def"}},
		{selector: "status=deployed", predicate: "status IN ($3)", args: []driver.Value{"deployed"}},
		{selector: "name=a", predicate: "name IN ($3)", args: []driver.Value{"a"}},
		{selector: "owner=helm", predicate: "owner IN ($3)", args: []driver.Value{"helm"}},
		{selector: "version=01", predicate: "version::text IN ($3)", args: []driver.Value{"01"}},
		{selector: "createdAt=0100", predicate: "createdAt::text IN ($3)", args: []driver.Value{"0100"}},
		{selector: "modifiedAt=0", predicate: "NULLIF(modifiedAt, 0)::text IN ($3)", args: []driver.Value{"0"}},
		{selector: "modifiedAt!=0", predicate: "(NULLIF(modifiedAt, 0)::text IS NULL OR NULLIF(modifiedAt, 0)::text NOT IN ($3))", args: []driver.Value{"0"}},
		{selector: "modifiedAt", predicate: "NULLIF(modifiedAt, 0)::text IS NOT NULL", args: nil},
		{selector: "!modifiedAt", predicate: "NULLIF(modifiedAt, 0)::text IS NULL", args: nil},
		{selector: "!version", predicate: "version::text IS NULL", args: nil},
		{selector: "version", predicate: "version::text IS NOT NULL", args: nil},
	}
	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			backend, mock := newTestSQLBackend(t)
			selector, err := labels.Parse(tt.selector)
			require.NoError(t, err)
			args := append([]driver.Value{"helm", testNamespace}, tt.args...)
			mock.ExpectQuery(testSQLLatestSelect + " AND namespace = $2 AND " + tt.predicate + testSQLLatestOrder).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"key"})).RowsWillBeClosed()
			require.NoError(t, backend.listLatestWithBodies(context.Background(), testNamespace, selector, func(*storedObject) error {
				t.Fatal("unexpected callback")

				return nil
			}))
		})
	}
}

func TestSQLStorageBackend_ListLatestWithBodies(t *testing.T) {
	backend, mock := newTestSQLBackend(t)
	backend.db.SetMaxOpenConns(1)
	mock.ExpectQuery(testSQLLatestSelect + testSQLLatestOrder).WithArgs("helm").WillReturnRows(
		sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat", "body", "custom_labels"}).
			AddRow("same-key", "ns-1", "a", 2, "deployed", "helm", 100, 0, "first", `{"packageChecksum":"one","status":"bogus","modifiedAt":"bogus"}`).
			AddRow("same-key", "ns-2", "a", 2, "deployed", "helm", 100, 200, "second", `{"packageChecksum":"two"}`)).RowsWillBeClosed()
	var objects []*storedObject
	require.NoError(t, backend.listLatestWithBodies(context.Background(), "", nil, func(obj *storedObject) error {
		objects = append(objects, obj)

		return nil
	}))
	require.Len(t, objects, 2)
	assert.Equal(t, "ns-1", objects[0].Namespace)
	assert.Equal(t, "one", objects[0].Labels["packageChecksum"])
	assert.Equal(t, "deployed", objects[0].Labels["status"])
	assert.NotContains(t, objects[0].Labels, "modifiedAt")
	assert.Equal(t, []byte("first"), objects[0].Body)
	assert.Equal(t, "ns-2", objects[1].Namespace)
	assert.Equal(t, "two", objects[1].Labels["packageChecksum"])
	assert.Equal(t, "200", objects[1].Labels["modifiedAt"])
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

func TestSQLStorageBackend_ListWithBodiesOfVersions(t *testing.T) {
	backend, mock := newTestSQLBackend(t)

	body := encodeHelmRelease(t, newTestReleaseWithStatus("myrel", 2, helmreleasecommon.StatusDeployed))

	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt FROM releases_v1 WHERE owner = $1 AND namespace = $2 AND name = $3 AND version IN ($4)`).
		WithArgs("helm", testNamespace, "myrel", 2).
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat"}).
			AddRow("sh.helm.release.v1.myrel.v2", testNamespace, "myrel", 2, "deployed", "helm", 200, 0))
	mock.ExpectQuery(`SELECT releaseKey, key, value FROM custom_labels_v1 WHERE releaseKey IN ($1) AND releaseNamespace = $2`).
		WithArgs("sh.helm.release.v1.myrel.v2", testNamespace).
		WillReturnRows(sqlmock.NewRows([]string{"releasekey", "key", "value"}))
	mock.ExpectQuery(`SELECT key, namespace, name, version, status, owner, createdAt, modifiedAt, body FROM releases_v1 WHERE owner = $1 AND namespace = $2 AND name = $3 AND version IN ($4) ORDER BY version ASC`).
		WithArgs("helm", testNamespace, "myrel", 2).
		WillReturnRows(sqlmock.NewRows([]string{"key", "namespace", "name", "version", "status", "owner", "createdat", "modifiedat", "body"}).
			AddRow("sh.helm.release.v1.myrel.v2", testNamespace, "myrel", 2, "deployed", "helm", 200, 0, string(body)))

	var keys []string
	require.NoError(t, backend.listWithBodies(context.Background(), testNamespace, "myrel", []int{2}, func(obj *storedObject) error {
		keys = append(keys, obj.Key)

		return nil
	}))

	assert.Equal(t, []string{"sh.helm.release.v1.myrel.v2"}, keys)
	require.NoError(t, mock.ExpectationsWereMet())
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
