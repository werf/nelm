package ts_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
)

var chartTSIgnoreEntries = []string{
	"ts/node_modules/",
	"ts/.dev/",
	"ts/.pnp.*",
	"ts/.yarn/cache/",
	"ts/.yarn/unplugged/",
	"ts/.yarn/__virtual__/",
	"ts/.yarn/install-state.gz",
	"ts/.yarn/build-state.yml",
}

func TestEnsureGitignore(t *testing.T) {
	t.Run("creates .gitignore if missing", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.EnsureGitignore(chartPath)
		require.NoError(t, err)

		assert.Equal(t, chartTSIgnoreEntries, ignoreFileEntries(t, filepath.Join(chartPath, ".gitignore")))
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
		assert.True(t, strings.HasPrefix(string(content), originalContent))
		assert.Equal(t, append([]string{"*.log"}, chartTSIgnoreEntries...), ignoreFileEntries(t, filepath.Join(chartPath, ".gitignore")))
	})

	t.Run("does not duplicate entries", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		existingContent := strings.Join(chartTSIgnoreEntries, "\n") + "\nts/dist/\n"
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

		assert.Equal(t, chartTSIgnoreEntries, ignoreFileEntries(t, filepath.Join(chartPath, ".gitignore")))
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

		assert.Equal(t, chartTSIgnoreEntries, ignoreFileEntries(t, filepath.Join(chartPath, ".helmignore")))
	})

	t.Run("appends missing TS entries to an existing .helmignore", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "ts-only-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(".git/\nts/node_modules/\n"), 0o644))

		err := ts.InitChartStructure(context.Background(), chartPath, "ts-only-chart")
		require.NoError(t, err)

		assert.Equal(t, append([]string{".git/"}, chartTSIgnoreEntries...), ignoreFileEntries(t, filepath.Join(chartPath, ".helmignore")))
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

		var paths []string
		require.NoError(t, filepath.WalkDir(filepath.Join(chartPath, "ts"), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.IsDir() {
				relPath, err := filepath.Rel(filepath.Join(chartPath, "ts"), path)
				if err != nil {
					return fmt.Errorf("get scaffold file path: %w", err)
				}

				paths = append(paths, filepath.ToSlash(relPath))
			}

			return nil
		}))
		assert.ElementsMatch(t, []string{
			"input.example.yaml",
			"package.json",
			"src/context.ts",
			"src/deployment.test.ts",
			"src/deployment.ts",
			"src/helpers.ts",
			"src/index.ts",
			"src/service.ts",
			"src/values.test.json",
			"tsconfig.app.json",
			"tsconfig.json",
			"tsconfig.test.json",
		}, paths)
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

		assert.Equal(t, "my-custom-chart-ts", packageJSON["name"])
		assert.Equal(t, "0.0.0", packageJSON["version"])
		assert.Equal(t, "module", packageJSON["type"])
		assert.Equal(t, true, packageJSON["private"])
		assert.Equal(t, map[string]any{
			"dev":       "nelm-chart-ts dev --input-file input.example.yaml",
			"typecheck": "tsc --build",
			"test":      `node --import=tsx --test "src/**/*.test.ts"`,
		}, packageJSON["scripts"])
		assert.Equal(t, map[string]any{"@nelm/chart-ts-sdk": "^0.2.0"}, packageJSON["dependencies"])
		assert.Equal(t, map[string]any{"@types/node": "^26.6.6", "tsx": "^4.23.15", "typescript": "~6.0.3"}, packageJSON["devDependencies"])
	})

	t.Run("names the package after the chart", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		require.NoError(t, ts.InitTSBoilerplate(context.Background(), chartPath, "My-Chart", ts.InitTSBoilerplateOptions{}))

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
		require.NoError(t, err)

		var packageJSON map[string]any
		require.NoError(t, json.Unmarshal(content, &packageJSON))
		assert.Equal(t, "my-chart-ts", packageJSON["name"])
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
			Files           []string       `json:"files"`
			References      []struct {
				Path string `json:"path"`
			} `json:"references"`
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
		assert.Empty(t, tsconfig.Files)
		require.Len(t, tsconfig.References, 2)
		assert.Equal(t, "./tsconfig.app.json", tsconfig.References[0].Path)
		assert.Equal(t, "./tsconfig.test.json", tsconfig.References[1].Path)
	})

	t.Run("uses RenderContext by default", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.InitTSBoilerplate(context.Background(), chartPath, "test-chart", ts.InitTSBoilerplateOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "index.ts"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "function generate")
		assert.Contains(t, string(content), "ChartContext")
		assert.Contains(t, string(content), "RenderResult")
		assert.Contains(t, string(content), "await render(generate)")
		assert.NotContains(t, string(content), "WerfRenderContext")
		content, err = os.ReadFile(filepath.Join(chartPath, "ts", "src", "context.ts"))
		require.NoError(t, err)
		assert.Equal(t, "export type { RenderContext as ChartContext } from '@nelm/chart-ts-sdk';\n", string(content))
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
			assert.Contains(t, string(content), "ChartContext", "file %s should use ChartContext", file)
			assert.NotContains(t, string(content), "import type { RenderContext }", "file %s should not import RenderContext", file)
		}

		content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "context.ts"))
		require.NoError(t, err)
		assert.Equal(t, "export type { WerfRenderContext as ChartContext } from '@nelm/chart-ts-sdk';\n", string(content))
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

	t.Run("tells what to do when ts/ directory already exists", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "test-chart")
		require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "ts"), 0o755))

		err := ts.InitChartStructure(context.Background(), chartPath, "test-chart")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "remove it to initialize the chart again")
		assert.Contains(t, err.Error(), "npm install")
		assert.NotContains(t, err.Error(), "chart ts init")
	})
}

func TestInitTSBoilerplateInput(t *testing.T) {
	for _, contextType := range []string{"RenderContext", "WerfRenderContext"} {
		t.Run(contextType, func(t *testing.T) {
			chartPath := t.TempDir()
			require.NoError(t, ts.InitTSBoilerplate(context.Background(), chartPath, "My-Chart", ts.InitTSBoilerplateOptions{
				RenderContextType: contextType,
			}))

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "input.example.yaml"))
			require.NoError(t, err)

			var input map[string]any
			require.NoError(t, yaml.Unmarshal(content, &input))
			chart := input["Chart"].(map[string]any)
			assert.Equal(t, "My-Chart", chart["Name"])
			assert.Equal(t, "My-Chart.enabled", chart["Condition"])
			assert.Equal(t, "My-Chart description", chart["Description"])
			assert.Equal(t, []any{"My-Chart"}, chart["Keywords"])
			assert.Equal(t, []any{"https://example.org/My-Chart"}, chart["Sources"])
			assert.Equal(t, "My-Chart", chart["Tags"])

			release := input["Release"].(map[string]any)
			assert.Equal(t, "My-Chart", release["Name"])
			assert.Equal(t, "My-Chart", release["Namespace"])

			values := input["Values"].(map[string]any)
			assert.Equal(t, "nginx", values["image"].(map[string]any)["repository"])
			assert.Equal(t, contextType == "WerfRenderContext", values["global"] != nil)

			if contextType == "WerfRenderContext" {
				global := values["global"].(map[string]any)
				assert.Equal(t, "myapp", global["werf"].(map[string]any)["name"])
			}

			assert.NotContains(t, string(content), "example-chart")

			content, err = os.ReadFile(filepath.Join(chartPath, "ts", "src", "values.test.json"))
			require.NoError(t, err)

			var testValues map[string]any
			require.NoError(t, json.Unmarshal(content, &testValues))

			if contextType == "WerfRenderContext" {
				require.Contains(t, testValues, "global")
				global := testValues["global"].(map[string]any)
				assert.Equal(t, "myapp", global["werf"].(map[string]any)["name"])
			} else {
				assert.Empty(t, testValues)
			}
		})
	}
}

func TestInitTSBoilerplateWithNode(t *testing.T) {
	if os.Getenv("NELM_TEST_TS_SCAFFOLD") != "1" {
		t.Skip("set NELM_TEST_TS_SCAFFOLD=1 to install dependencies and check the generated scaffold")
	}

	taskPath, err := exec.LookPath("task")
	require.NoError(t, err)
	taskfilePath, err := filepath.Abs(filepath.Join("..", "..", "Taskfile.dist.yaml"))
	require.NoError(t, err)

	for _, contextType := range []string{"RenderContext", "WerfRenderContext"} {
		t.Run(contextType, func(t *testing.T) {
			chartPath := t.TempDir()
			require.NoError(t, ts.InitTSBoilerplate(context.Background(), chartPath, "generated-chart", ts.InitTSBoilerplateOptions{
				RenderContextType: contextType,
			}))
			assert.NoFileExists(t, filepath.Join(chartPath, "Chart.yaml"))
			assert.NoFileExists(t, filepath.Join(chartPath, "ts", "package-lock.json"))

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			cmd := exec.CommandContext(ctx, taskPath, "--taskfile", taskfilePath, "test:ts-scaffold", "dir="+filepath.Join(chartPath, "ts"))
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			assert.Contains(t, string(output), "uses defaults with empty values")
			_, rendered, found := strings.Cut(string(output), "Rendered manifests:\n")
			require.True(t, found, "%s", output)

			names := make(map[string]string)

			decoder := yaml.NewDecoder(strings.NewReader(rendered))
			for {
				var manifest struct {
					Kind     string `yaml:"kind"`
					Metadata struct {
						Name string `yaml:"name"`
					} `yaml:"metadata"`
				}

				err := decoder.Decode(&manifest)
				if errors.Is(err, io.EOF) {
					break
				}

				require.NoError(t, err)

				names[manifest.Kind] = manifest.Metadata.Name
			}

			assert.Equal(t, map[string]string{"Deployment": "generated-chart", "Service": "generated-chart"}, names)
			checkScaffoldEditor(t, filepath.Join(chartPath, "ts"))
		})
	}
}

func ignoreFileEntries(t *testing.T, path string) []string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return lo.Filter(strings.Split(string(content), "\n"), func(line string, _ int) bool {
		return line != "" && !strings.HasPrefix(line, "#")
	})
}
