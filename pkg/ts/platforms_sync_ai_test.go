//go:build ai_tests

package ts_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/ts/denolock"
)

// A platform missing from any of the three lists is a release binary that silently falls back to
// downloading Deno at run time.
func TestAI_EmbedPlatformListsInSync(t *testing.T) {
	embedFiles, err := filepath.Glob("embed_*.go")
	require.NoError(t, err)

	fileRe := regexp.MustCompile(`^embed_([a-z0-9]+)_([a-z0-9]+)\.go$`)

	var embedPlatforms []string
	for _, f := range embedFiles {
		m := fileRe.FindStringSubmatch(filepath.Base(f))
		require.NotNil(t, m, "unexpected embed file name: %s", f)
		embedPlatforms = append(embedPlatforms, m[1]+"/"+m[2])
	}

	require.NotEmpty(t, embedPlatforms)
	sort.Strings(embedPlatforms)

	lock, err := denolock.Read()
	require.NoError(t, err)

	lockPlatforms := slices.Sorted(maps.Keys(lock.Platforms))

	taskfileSrc, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.dist.yaml"))
	require.NoError(t, err)

	taskfileRe := regexp.MustCompile(`platform: "([a-z0-9]+)/([a-z0-9]+)"`)

	var taskfilePlatforms []string
	for _, m := range taskfileRe.FindAllStringSubmatch(string(taskfileSrc), -1) {
		taskfilePlatforms = append(taskfilePlatforms, m[1]+"/"+m[2])
	}

	sort.Strings(taskfilePlatforms)

	assert.Equal(t, embedPlatforms, lockPlatforms,
		"pkg/ts/denolock/data/lock.json platforms out of sync with pkg/ts/embed_*.go files, run: task deno:lock")
	assert.Equal(t, embedPlatforms, taskfilePlatforms,
		"Taskfile.dist.yaml deno:embed platforms out of sync with pkg/ts/embed_*.go files")
}
