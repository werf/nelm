package schemas_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource/schemas"
)

func TestCRDsSource(t *testing.T) {
	t.Run("unpacks a schema per group directory", func(t *testing.T) {
		setupCacheHome(t)

		source := requireCRDsSource(t)
		require.NoError(t, source.EnsureExtracted(context.Background()))

		dir := sourceDir(t, source)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.NotEmpty(t, entries)

		for _, entry := range entries {
			require.True(t, entry.IsDir(), "the CRDs bundle must unpack into one directory per API group")
		}

		// A well known CRD, laid out the way the schema path template resolves.
		assert.FileExists(t, filepath.Join(dir, "monitoring.coreos.com", "prometheus_v1.json"))
	})

	t.Run("template matches the upstream catalog layout", func(t *testing.T) {
		setupCacheHome(t)

		source := requireCRDsSource(t)

		assert.True(t, strings.HasSuffix(source.Template, ".json"), "got %s", source.Template)
		assert.Contains(t, filepath.ToSlash(source.Template), "{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json")
	})
}

func TestEmbeddedBundlesPresent(t *testing.T) {
	// Reading the index is what rejects a binary that cannot validate resources on its own, so this
	// failing means the committed bundles are unusable: run task generate:validation-schemas.
	index, err := schemas.ReadIndex()
	require.NoError(t, err)

	require.NotNil(t, index.Kubernetes, "the Kubernetes schemas bundle is missing")
	assert.Positive(t, index.Kubernetes.FilesCount)
	assert.Positive(t, index.Kubernetes.UncompressedSize)
	assert.NotEmpty(t, index.Kubernetes.UpstreamCommit)

	kubeVersions := index.Kubernetes.KubeVersions
	require.NotEmpty(t, kubeVersions, "the Kubernetes bundle records no versions it was built from")
	assert.Equal(t, kubeVersions, lo.Uniq(kubeVersions), "every collected version must be distinct")

	// The newest collected version is the one resources are validated against. That it is the version
	// pinned in the Taskfile is what "task lint:validation-schemas" checks.
	kubeVersion, err := schemas.KubeVersion()
	require.NoError(t, err)
	assert.Equal(t, kubeVersions[0], kubeVersion)

	// Newest first: the merge relies on that order, since a version only contributes the schemas that
	// none of the versions before it in this list has.
	for i := 1; i < len(kubeVersions); i++ {
		assert.Less(t, minorNumberOf(t, kubeVersions[i]), minorNumberOf(t, kubeVersions[i-1]),
			"the collected versions must be recorded newest first, got %v", kubeVersions)
	}

	require.NotNil(t, index.CRDs, "the CRDs bundle is missing")
	assert.Positive(t, index.CRDs.FilesCount)
	assert.Positive(t, index.CRDs.UncompressedSize)
	assert.NotEmpty(t, index.CRDs.UpstreamCommit)
	assert.Empty(t, index.CRDs.KubeVersions, "CRD schemas are not tied to a Kubernetes version")
}

func TestEnsureExtracted(t *testing.T) {
	t.Run("is idempotent", func(t *testing.T) {
		setupCacheHome(t)

		ctx := context.Background()
		source := requireKubernetesSource(t)

		require.NoError(t, source.EnsureExtracted(ctx))

		marker := filepath.Join(sourceDir(t, source), "deployment-apps-v1.json")

		stat, err := os.Stat(marker)
		require.NoError(t, err)

		require.NoError(t, source.EnsureExtracted(ctx))

		// Unpacking again would have recreated the file.
		restat, err := os.Stat(marker)
		require.NoError(t, err)
		assert.Equal(t, stat.ModTime(), restat.ModTime())
	})

	t.Run("is safe to call concurrently", func(t *testing.T) {
		setupCacheHome(t)

		ctx := context.Background()
		source := requireKubernetesSource(t)
		errs := make([]error, 8)

		var wg sync.WaitGroup

		for i := range errs {
			wg.Add(1)

			go func() {
				defer wg.Done()

				errs[i] = source.EnsureExtracted(ctx)
			}()
		}

		wg.Wait()

		for i := range errs {
			require.NoError(t, errs[i])
		}
	})

	t.Run("unpacks bundles side by side under the nelm cache directory", func(t *testing.T) {
		cacheHome := setupCacheHome(t)

		ctx := context.Background()
		kubernetesSource := requireKubernetesSource(t)
		crdsSource := requireCRDsSource(t)

		require.NoError(t, kubernetesSource.EnsureExtracted(ctx))
		require.NoError(t, crdsSource.EnsureExtracted(ctx))

		kubernetesDir := sourceDir(t, kubernetesSource)
		crdsDir := sourceDir(t, crdsSource)

		assert.NotEqual(t, kubernetesDir, crdsDir)

		for _, dir := range []string{kubernetesDir, crdsDir} {
			assert.True(t, strings.HasPrefix(dir, cacheHome), "expected %s to be under the cache home %s", dir, cacheHome)
			assert.Contains(t, filepath.ToSlash(dir), common.CacheDirEmbeddedAPIResourceJSONSchemas)
		}
	})
}

func TestKubernetesSource(t *testing.T) {
	t.Run("unpacks a flat directory of schemas", func(t *testing.T) {
		setupCacheHome(t)

		source := requireKubernetesSource(t)
		require.NoError(t, source.EnsureExtracted(context.Background()))

		dir := sourceDir(t, source)
		index, err := schemas.ReadIndex()
		require.NoError(t, err)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, index.Kubernetes.FilesCount)

		for _, entry := range entries {
			require.False(t, entry.IsDir(), "the Kubernetes bundle must unpack flat")
		}

		// The file kubeconform builds a path to for Deployment/apps/v1 must be there.
		assert.FileExists(t, filepath.Join(dir, "deployment-apps-v1.json"))
	})

	t.Run("template resolves without a Kubernetes version in the path", func(t *testing.T) {
		setupCacheHome(t)

		source := requireKubernetesSource(t)

		// Ending with ".json" is what stops kubeconform from treating the source as a directory and
		// appending its own "<version>-standalone/..." path suffix to it.
		assert.True(t, strings.HasSuffix(source.Template, ".json"), "got %s", source.Template)
		assert.Contains(t, source.Template, "{{ .ResourceKind }}{{ .KindSuffix }}")
		assert.NotContains(t, source.Template, "standalone")
	})
}

// minorNumberOf returns the minor of a Kubernetes version as a number, so that versions can be
// ordered by it.
func minorNumberOf(t *testing.T, kubeVersion string) int {
	t.Helper()

	parts := strings.SplitN(strings.TrimPrefix(kubeVersion, "v"), ".", 3)
	require.GreaterOrEqual(t, len(parts), 2, "kube version %q", kubeVersion)

	minor, err := strconv.Atoi(parts[1])
	require.NoError(t, err, "kube version %q", kubeVersion)

	return minor
}

func requireCRDsSource(t *testing.T) *schemas.Source {
	t.Helper()

	source, err := schemas.CRDsSource()
	require.NoError(t, err)
	require.NotNil(t, source)

	return source
}

func requireKubernetesSource(t *testing.T) *schemas.Source {
	t.Helper()

	source, err := schemas.KubernetesSource()
	require.NoError(t, err)
	require.NotNil(t, source)

	return source
}

// setupCacheHome points the cache directory, which is where nelm unpacks the embedded schemas, at a
// temporary directory, and returns it. Note that the vendored helmpath only honors XDG_CACHE_HOME,
// not HELM_CACHE_HOME.
func setupCacheHome(t *testing.T) string {
	t.Helper()

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	return cacheHome
}

// sourceDir is the directory a source's path template resolves against.
func sourceDir(t *testing.T, source *schemas.Source) string {
	t.Helper()

	dir, _, found := strings.Cut(source.Template, "{{")
	require.True(t, found, "source template %s has no placeholders", source.Template)

	return filepath.Clean(dir)
}
