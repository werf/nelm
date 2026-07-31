//go:build ai_tests

package ts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	generatorSrc, err := os.ReadFile(filepath.Join("..", "..", "cmd", "embed-deno", "main.go"))
	require.NoError(t, err)

	generatorRe := regexp.MustCompile(`\{"([a-z0-9]+)", "([a-z0-9]+)"\}`)

	var generatorPlatforms []string
	for _, m := range generatorRe.FindAllStringSubmatch(string(generatorSrc), -1) {
		generatorPlatforms = append(generatorPlatforms, m[1]+"/"+m[2])
	}

	sort.Strings(generatorPlatforms)

	taskfileSrc, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.dist.yaml"))
	require.NoError(t, err)

	taskfileRe := regexp.MustCompile(`platform: "([a-z0-9]+)/([a-z0-9]+)"`)

	var taskfilePlatforms []string
	for _, m := range taskfileRe.FindAllStringSubmatch(string(taskfileSrc), -1) {
		taskfilePlatforms = append(taskfilePlatforms, m[1]+"/"+m[2])
	}

	sort.Strings(taskfilePlatforms)

	assert.Equal(t, embedPlatforms, generatorPlatforms, "cmd/embed-deno platforms whitelist out of sync with pkg/ts/embed_*.go files")
	assert.Equal(t, embedPlatforms, taskfilePlatforms, "Taskfile.dist.yaml deno:embed platforms out of sync with pkg/ts/embed_*.go files")
}
