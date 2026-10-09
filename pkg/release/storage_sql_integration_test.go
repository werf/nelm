package release

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

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

	// Registered after the delete cleanup, so it runs first and releases the row lock if the
	// test fails while holding it.
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
