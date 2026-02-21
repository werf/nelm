package ts_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/internal/ts"
)

func TestEnsureGitignore(t *testing.T) {
	t.Run("creates .gitignore if missing", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.EnsureGitignore(chartPath)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "ts/node_modules/")
		assert.Contains(t, string(content), "ts/vendor/")
		assert.Contains(t, string(content), "ts/dist/")
	})

	t.Run("appends missing entries to existing .gitignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		originalContent := "# My project\n*.log\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(originalContent), 0o644))

		err := ts.EnsureGitignore(chartPath)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "# My project")
		assert.Contains(t, string(content), "*.log")
		assert.Contains(t, string(content), "ts/node_modules/")
		assert.Contains(t, string(content), "ts/vendor/")
		assert.Contains(t, string(content), "ts/dist/")
	})

	t.Run("does not duplicate entries", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := "ts/node_modules/\nts/vendor/\nts/dist/\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0o644))

		err := ts.EnsureGitignore(chartPath)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content))
	})

	t.Run("adds only missing entries", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := "ts/node_modules/\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0o644))

		err := ts.EnsureGitignore(chartPath)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "ts/node_modules/")
		assert.Contains(t, string(content), "ts/vendor/")
		assert.Contains(t, string(content), "ts/dist/")
	})
}

func TestInitChartStructure(t *testing.T) {
	t.Run("creates Chart.yaml, values.yaml, .helmignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		assert.FileExists(t, filepath.Join(chartPath, "Chart.yaml"))
		assert.FileExists(t, filepath.Join(chartPath, "values.yaml"))
		assert.FileExists(t, filepath.Join(chartPath, ".helmignore"))
	})

	t.Run("does not create charts/ directory", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		assert.NoDirExists(t, filepath.Join(chartPath, "charts"))
	})

	t.Run("does not create templates/ directory", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		assert.NoDirExists(t, filepath.Join(chartPath, "templates"))
	})

	t.Run("substitutes chart name in Chart.yaml", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-ts-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "my-ts-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "name: my-ts-chart")
	})

	t.Run("includes TS entries in .helmignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "ts/vendor/")
		assert.Contains(t, string(content), "ts/node_modules/")
	})

	t.Run("skips existing Chart.yaml", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "existing-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := "apiVersion: v2\nname: existing-name\nversion: 1.0.0\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte(existingContent), 0o644))

		err := ts.InitChartStructure(context.Background(), chartPath, "new-name")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content))
	})

	t.Run("skips existing values.yaml", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "existing-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := "myValue: 123\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, "values.yaml"), []byte(existingContent), 0o644))

		err := ts.InitChartStructure(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "values.yaml"))
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content))
	})

	t.Run("enriches existing .helmignore with TS entries", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "existing-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := ".DS_Store\n.git/\n"
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(existingContent), 0o644))

		err := ts.InitChartStructure(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), ".DS_Store")
		assert.Contains(t, string(content), ".git/")
		assert.Contains(t, string(content), "ts/vendor/")
		assert.Contains(t, string(content), "ts/node_modules/")
	})
}

func TestInitTSBoilerplate(t *testing.T) {
	t.Run("creates all expected files", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		// Check ts/src/ files
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "index.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "helpers.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "deployment.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "service.ts"))

		// Check ts/ root files
		assert.FileExists(t, filepath.Join(chartPath, "ts", "tsconfig.json"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "deno.json"))
	})

	t.Run("creates correct directory structure", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		assert.DirExists(t, filepath.Join(chartPath, "ts"))
		assert.DirExists(t, filepath.Join(chartPath, "ts", "src"))
	})

	t.Run("includes correct deno.json config", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-custom-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "my-custom-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "deno.json"))
		require.NoError(t, err)
		assert.Contains(t, string(content), fmt.Sprintf(`"build": "%s"`, ts.ChartTSBuildScript))
		assert.Contains(t, string(content), `"@nelm/chart-ts-sdk"`)
	})

	t.Run("includes render function in index.ts", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "index.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "function render")
		assert.Contains(t, string(content), "RenderContext")
		assert.Contains(t, string(content), "RenderResult")
		assert.Contains(t, string(content), "runRender")
	})

	t.Run("includes helper functions in helpers.ts", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "helpers.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "export function getFullname")
		assert.Contains(t, string(content), "export function getLabels")
		assert.Contains(t, string(content), "export function getSelectorLabels")
	})

	t.Run("includes resource generators", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		deploymentContent, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "deployment.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(deploymentContent), "export function newDeployment")

		serviceContent, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "service.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(serviceContent), "export function newService")
	})

	t.Run("includes @nelm/chart-ts-sdk dependency", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "deno.json"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `"@nelm/chart-ts-sdk"`)
	})

	t.Run("includes correct tsconfig.json options", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "tsconfig.json"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `"target": "ES2015"`)
		assert.Contains(t, string(content), `"module": "CommonJS"`)
		assert.Contains(t, string(content), `"strict": true`)
		assert.Contains(t, string(content), `"declaration": true`)
	})

	t.Run("fails if ts/ directory already exists", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		tsDir := filepath.Join(chartPath, "ts")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "typescript directory already exists")
	})
}
