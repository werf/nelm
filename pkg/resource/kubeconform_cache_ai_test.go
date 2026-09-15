//go:build ai_tests

package resource_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource"
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type testCacheMetadata struct {
	APIVersion string `json:"apiVersion"`
	Entries    map[string]struct {
		Created    time.Time `json:"created"`
		SchemaFile string    `json:"schemaFile"`
	} `json:"entries"`
}

func TestAI_SchemaCacheEntryOfPreviousNelmVersion(t *testing.T) {
	cacheHome := setupTestEnvironmentWithCacheHome(t)

	server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

	ctx := context.Background()
	deployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)
	opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

	require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

	cacheDir := findRemoteSchemaCacheDir(t, cacheHome)

	// Older nelm versions did not record the schema file name. Such an entry must still expire
	// cleanly instead of failing the run, even though its schema cannot be dropped from disk.
	metadataPath := filepath.Join(cacheDir, "metadata.json")
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(metadataBytes, &raw))

	for _, entry := range raw["entries"].(map[string]any) {
		delete(entry.(map[string]any), "schemaFile")
	}

	strippedBytes, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metadataPath, strippedBytes, 0o644))

	expiringOpts := makeValidationOptions([]string{server.URL + schemaURLTemplate})
	expiringOpts.ValidationSchemaCacheLifetime = time.Nanosecond

	require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, expiringOpts))

	// The entry is rewritten by the current version, so from now on it can be evicted properly.
	for _, entry := range readTestCacheMetadata(t, cacheDir).Entries {
		assert.NotEmpty(t, entry.SchemaFile)
	}
}

func TestAI_SchemaCacheInvalidation(t *testing.T) {
	t.Run("records the file kubeconform cached each schema in", func(t *testing.T) {
		cacheHome := setupTestEnvironmentWithCacheHome(t)

		server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{
			makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace),
			makeInstallableResource(t, validConfigMapObject(), testReleaseNamespace),
		}, opts))

		cacheDir := findRemoteSchemaCacheDir(t, cacheHome)
		metadata := readTestCacheMetadata(t, cacheDir)

		require.Len(t, metadata.Entries, 2, "one entry per validated GroupVersionKind")

		// This is the whole point: the recorded name must be the name kubeconform actually used,
		// otherwise expiry deletes nothing. Deriving it wrongly would leave these files unmatched.
		for hash, entry := range metadata.Entries {
			require.NotEmpty(t, entry.SchemaFile, "entry %s recorded no schema file", hash)
			assert.FileExists(t, filepath.Join(cacheDir, entry.SchemaFile))
		}

		onDisk := cachedSchemaFiles(t, cacheDir)
		require.Len(t, onDisk, 2, "kubeconform should have cached both schemas")

		recorded := make([]string, 0, len(metadata.Entries))
		for _, entry := range metadata.Entries {
			recorded = append(recorded, entry.SchemaFile)
		}

		assert.ElementsMatch(t, onDisk, recorded, "every cached schema must be accounted for in the metadata")
	})

	t.Run("records the file for a source that has no path template", func(t *testing.T) {
		cacheHome := setupTestEnvironmentWithCacheHome(t)

		// kubeconform appends its own "<version>-standalone/<kind><suffix>.json" layout to any source
		// that does not already point at a file, so this bare URL ends up serving the same schemas. The
		// name of the cached file has to be derived from the location kubeconform actually built, not
		// from the source as configured, or the entry expiry has nothing to delete.
		server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL})

		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace,
			[]*resource.InstallableResource{makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)}, opts))

		cacheDir := findRemoteSchemaCacheDir(t, cacheHome)
		metadata := readTestCacheMetadata(t, cacheDir)

		require.Len(t, metadata.Entries, 1)

		for hash, entry := range metadata.Entries {
			require.NotEmpty(t, entry.SchemaFile, "entry %s recorded no schema file", hash)
			assert.FileExists(t, filepath.Join(cacheDir, entry.SchemaFile))
		}

		assert.ElementsMatch(t, cachedSchemaFiles(t, cacheDir),
			[]string{firstSchemaFile(t, metadata)}, "the cached schema must be the one the entry names")
	})

	t.Run("expired entries drop the cached schema and it is downloaded again", func(t *testing.T) {
		cacheHome := setupTestEnvironmentWithCacheHome(t)

		server, downloadCount := setupSchemaServerCountingDownloads(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

		ctx := context.Background()
		deployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})
		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

		cacheDir := findRemoteSchemaCacheDir(t, cacheHome)
		require.Len(t, cachedSchemaFiles(t, cacheDir), 1)
		require.Equal(t, 1, *downloadCount)

		// Everything is stale now, so the cached schema must be dropped and fetched anew.
		expiringOpts := makeValidationOptions([]string{server.URL + schemaURLTemplate})
		expiringOpts.ValidationSchemaCacheLifetime = time.Nanosecond

		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, expiringOpts))

		assert.Equal(t, 2, *downloadCount, "the expired schema must have been downloaded again")
		assert.Len(t, cachedSchemaFiles(t, cacheDir), 1, "the re-downloaded schema must be cached again")
	})

	t.Run("live entries keep the cached schema and skip downloading", func(t *testing.T) {
		cacheHome := setupTestEnvironmentWithCacheHome(t)

		server, downloadCount := setupSchemaServerCountingDownloads(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

		ctx := context.Background()
		deployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

		cacheDir := findRemoteSchemaCacheDir(t, cacheHome)
		cachedBefore := cachedSchemaFiles(t, cacheDir)

		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

		assert.ElementsMatch(t, cachedBefore, cachedSchemaFiles(t, cacheDir), "a live entry must not be evicted")
		assert.Equal(t, 1, *downloadCount, "a live entry must not be downloaded again")
	})
}

func TestAI_SchemaCacheMetadataRecovery(t *testing.T) {
	// The metadata is only an optimization, so none of these may fail a run. Before, each of them
	// made validation impossible until the cache was wiped by hand.
	for _, tt := range []struct {
		content string
		name    string
	}{
		{content: "", name: "empty file"},
		{content: `{"apiVersion":"v1","entries":{"abc":{"created":`, name: "truncated mid-write"},
		{content: "not json at all", name: "not json"},
		{content: `{"apiVersion":"v99","entries":{}}`, name: "unsupported format version"},
		{content: `{"apiVersion":"v1"}`, name: "no entries key at all"},
		{content: `{}`, name: "empty object"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cacheHome := setupTestEnvironmentWithCacheHome(t)

			server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

			ctx := context.Background()
			deployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)
			opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

			require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

			cacheDir := findRemoteSchemaCacheDir(t, cacheHome)
			metadataPath := filepath.Join(cacheDir, "metadata.json")
			require.NoError(t, os.WriteFile(metadataPath, []byte(tt.content), 0o644))

			require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts),
				"a broken cache must not fail validation")

			// The broken file is replaced by a usable one, so the cache works again from now on.
			metadata := readTestCacheMetadata(t, cacheDir)
			assert.Equal(t, "v1", metadata.APIVersion)
			assert.Len(t, metadata.Entries, 1)
		})
	}
}

func TestAI_SchemaCacheMetadataWrittenAtomically(t *testing.T) {
	cacheHome := setupTestEnvironmentWithCacheHome(t)

	server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

	ctx := context.Background()
	deployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)
	opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

	require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts))

	// No staging file may be left behind next to the metadata it was written through.
	entries, err := os.ReadDir(findRemoteSchemaCacheDir(t, cacheHome))
	require.NoError(t, err)

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp-", "temp file %s was not cleaned up", entry.Name())
	}
}

// cachedSchemaFiles returns the schemas kubeconform has cached in dir, skipping nelm's own metadata
// and lock files.
func cachedSchemaFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var names []string

	for _, entry := range entries {
		if !entry.IsDir() && isCachedSchemaName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}

	return names
}

// findRemoteSchemaCacheDir locates the cache dir of the only remote schema source in play.
func findRemoteSchemaCacheDir(t *testing.T, cacheHome string) string {
	t.Helper()

	root := filepath.Join(cacheHome, "helm", filepath.FromSlash(common.CacheDirAPIResourceJSONSchemas))
	embeddedRoot := filepath.Join(cacheHome, "helm", filepath.FromSlash(common.CacheDirEmbeddedAPIResourceJSONSchemas))

	var found []string

	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// The unpacked embedded bundles live under this root too, and are none of this helper's
		// business.
		if path == embeddedRoot {
			return filepath.SkipDir
		}

		// Local file system sources, the embedded bundles among them, get a "local-" prefixed dir
		// and never cache anything into it. The remaining level is the per-source-set hash dir.
		if entry.IsDir() && entry.Name() != filepath.Base(root) &&
			!isCachedSchemaName(entry.Name()) && !strings.HasPrefix(entry.Name(), "local-") {
			found = append(found, path)
		}

		return nil
	}))

	require.Len(t, found, 1, "expected exactly one remote source cache dir, got %v", found)

	return found[0]
}

func firstSchemaFile(t *testing.T, metadata testCacheMetadata) string {
	t.Helper()

	for _, entry := range metadata.Entries {
		return entry.SchemaFile
	}

	t.Fatal("metadata has no entries")

	return ""
}

// isCachedSchemaName reports whether name is a sha256 hash, which is how kubeconform names the
// files it caches downloaded schemas in, and how nelm names the per-source cache directories. It
// tells both apart from nelm's own metadata.json and lock.
func isCachedSchemaName(name string) bool {
	return sha256HexPattern.MatchString(name)
}

// kubeConformNamedSchemas serves the test schemas under the names kubeconform actually requests.
func kubeConformNamedSchemas(t *testing.T, kubeVersion string) map[string]string {
	t.Helper()

	prefix := "v" + kubeVersion + "-standalone/"

	return map[string]string{
		prefix + "deployment-apps-v1.json": loadSchema(t, "deployment"),
		prefix + "configmap-v1.json":       loadSchema(t, "configmap"),
	}
}

func readTestCacheMetadata(t *testing.T, dir string) testCacheMetadata {
	t.Helper()

	metadataBytes, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err)

	var metadata testCacheMetadata

	require.NoError(t, json.Unmarshal(metadataBytes, &metadata))

	return metadata
}

// setupSchemaServerCountingDownloads counts only the GETs that actually fetch a schema, so that a
// request of any other kind cannot be mistaken for a cache miss.
func setupSchemaServerCountingDownloads(t *testing.T, schemas map[string]string) (*httptest.Server, *int) {
	t.Helper()

	downloadCount := new(int)
	baseHandler := newSchemaHandler(schemas)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			*downloadCount++
		}

		baseHandler.ServeHTTP(w, r)
	}))

	t.Cleanup(server.Close)

	return server, downloadCount
}

// setupTestEnvironmentWithCacheHome is setupTestEnvironment that also hands back the cache home, so
// that tests can look into what ended up on disk.
func setupTestEnvironmentWithCacheHome(t *testing.T) string {
	t.Helper()

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	return cacheHome
}

func validConfigMapObject() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "test-configmap",
		},
		"data": map[string]interface{}{
			"key": "value",
		},
	}
}
