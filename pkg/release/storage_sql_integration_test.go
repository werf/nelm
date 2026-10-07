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

func TestSQLStorageBackend_SelectorsPostgres(t *testing.T) {
	connection := os.Getenv("NELM_TEST_POSTGRES_CONNECTION")
	if connection == "" {
		t.Skip("NELM_TEST_POSTGRES_CONNECTION is not set")
	}

	ctx := context.Background()
	b, err := newSQLStorageBackend(ctx, connection)
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
