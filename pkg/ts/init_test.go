package ts_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
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
		assert.Contains(t, string(content), "ts/.dev/")
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
		assert.Contains(t, string(content), "ts/.dev/")
	})

	t.Run("does not duplicate entries", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := "ts/node_modules/\nts/.dev/\nts/dist/\n"
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
		assert.Contains(t, string(content), "ts/.dev/")
	})
}

func TestInitChartStructure(t *testing.T) {
	t.Run("creates Chart.yaml, values.yaml, .helmignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

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

	t.Run("includes TS entries in .helmignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "ts/node_modules/")
		assert.Contains(t, string(content), "ts/.dev/")
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
		assert.Contains(t, string(content), "ts/node_modules/")
		assert.Contains(t, string(content), "ts/.dev/")
	})
}

func TestInitTSBoilerplate(t *testing.T) {
	t.Run("creates all expected files", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "index.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "helpers.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "deployment.ts"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "src", "service.ts"))

		assert.FileExists(t, filepath.Join(chartPath, "ts", "package.json"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "tsconfig.json"))
		assert.FileExists(t, filepath.Join(chartPath, "ts", "input.example.yaml"))
	})

	t.Run("creates correct directory structure", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		assert.DirExists(t, filepath.Join(chartPath, "ts"))
		assert.DirExists(t, filepath.Join(chartPath, "ts", "src"))
	})

	t.Run("includes correct package.json", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-custom-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "my-custom-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
		require.NoError(t, err)

		var packageJSON map[string]any
		require.NoError(t, json.Unmarshal(content, &packageJSON))

		assert.Equal(t, "module", packageJSON["type"])
		assert.Equal(t, true, packageJSON["private"])
		assert.Equal(t, map[string]any{
			"dev":       "nelm-chart-ts dev --input-file input.example.yaml",
			"typecheck": "tsc",
		}, packageJSON["scripts"])
		assert.Equal(t, map[string]any{"@nelm/chart-ts-sdk": "^0.2.0"}, packageJSON["dependencies"])
		assert.Contains(t, packageJSON["devDependencies"], "typescript")
	})

	t.Run("includes correct tsconfig.json", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "tsconfig.json"))
		require.NoError(t, err)

		var tsconfig struct {
			CompilerOptions map[string]any `json:"compilerOptions"`
			Include         []string       `json:"include"`
		}
		require.NoError(t, json.Unmarshal(content, &tsconfig))

		assert.Equal(t, map[string]any{
			"target":                     "ES2022",
			"module":                     "ESNext",
			"moduleResolution":           "bundler",
			"allowImportingTsExtensions": true,
			"noEmit":                     true,
			"isolatedModules":            true,
			"verbatimModuleSyntax":       true,
			"strict":                     true,
			"skipLibCheck":               true,
			"types":                      []any{},
		}, tsconfig.CompilerOptions)
		assert.Equal(t, []string{"src"}, tsconfig.Include)
	})

	t.Run("uses RenderContext by default", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "index.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "function generate")
		assert.Contains(t, string(content), "RenderContext")
		assert.Contains(t, string(content), "RenderResult")
		assert.Contains(t, string(content), "await render(generate)")
		assert.NotContains(t, string(content), "WerfRenderContext")
	})

	t.Run("uses custom render context type when specified", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{
			RenderContextType: "WerfRenderContext",
		})
		require.NoError(t, err)

		for _, file := range []string{"index.ts", "helpers.ts", "deployment.ts", "service.ts"} {
			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", file))
			require.NoError(t, err)
			assert.Contains(t, string(content), "WerfRenderContext", "file %s should use WerfRenderContext", file)
			assert.NotContains(t, string(content), "import type { RenderContext }", "file %s should not import RenderContext", file)
		}
	})

	t.Run("includes helper functions in helpers.ts", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
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

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
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

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `"@nelm/chart-ts-sdk"`)
	})

	t.Run("includes chart name in input.example.yaml", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-custom-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "my-custom-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "input.example.yaml"))
		require.NoError(t, err)

		s := string(content)
		assert.Contains(t, s, "Name: my-custom-chart")
		assert.Contains(t, s, "Namespace: my-custom-chart")
		assert.Contains(t, s, "Values:")
		assert.Contains(t, s, "Capabilities:")
		assert.NotContains(t, s, "global:")
		assert.NotContains(t, s, "werf:")
	})

	t.Run("includes werf values in input.example.yaml for WerfRenderContext", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-werf-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "my-werf-chart", ts.InitTSBoilerplateOptions{
			RenderContextType: "WerfRenderContext",
		})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "input.example.yaml"))
		require.NoError(t, err)

		s := string(content)
		assert.Contains(t, s, "Name: my-werf-chart")
		assert.Contains(t, s, "global:")
		assert.Contains(t, s, "werf:")
		assert.Contains(t, s, "images:")
	})

	t.Run("fails if ts/ directory already exists", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		tsDir := filepath.Join(chartPath, "ts")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "typescript directory already exists")
	})
}
