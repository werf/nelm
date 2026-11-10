package ts_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/ts"
)

func TestRenderChartRemovesRenderFiles(t *testing.T) {
	skipWithoutShell(t)

	chartDir := copyTestChart(t)
	chart := loadChart(t, chartDir)
	require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, false))

	for name, test := range map[string]struct {
		body    string
		wantErr bool
	}{
		"after success": {body: fakeNodeScript},
		"after failure": {body: "#!/bin/sh\nexit 1\n", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			nodePath := filepath.Join(t.TempDir(), "node")
			require.NoError(t, os.WriteFile(nodePath, []byte(test.body), 0o755))

			tempDir := t.TempDir()
			values := chartcommon.Values{"Values": map[string]any{"password": "secret"}}

			_, err := ts.RenderChartRecursive(context.Background(), chart, values, chart.Name(), chartDir, tempDir, nodePath)
			assert.Equal(t, test.wantErr, err != nil, "error: %v", err)

			var leftovers []string
			require.NoError(t, filepath.WalkDir(tempDir, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() {
					leftovers = append(leftovers, path)
				}

				return err
			}))
			assert.Empty(t, leftovers, "the input with the values and the output must be removed")
		})
	}
}

func TestRenderChartWithNode(t *testing.T) {
	nodePath := requireTestNode(t)

	chartDir := copyTestChart(t)
	chart := loadChart(t, chartDir)

	rendered, err := ts.RenderChart(context.Background(), chart, chartcommon.Values{
		"Release": map[string]any{"Name": "my-release"},
		"Values":  map[string]any{"replicas": 2},
	}, false, chartDir, t.TempDir(), nodePath)
	require.NoError(t, err)

	require.Contains(t, rendered, "ts-chart/ts/src/index.ts")
	require.Contains(t, rendered, "ts-chart/charts/subchart/ts/src/index.js")

	var manifest struct {
		Data struct {
			Input string `json:"input"`
		} `json:"data"`
		Kind string `json:"kind"`
	}

	require.NoError(t, yaml.Unmarshal([]byte(rendered["ts-chart/ts/src/index.ts"]), &manifest))
	assert.Equal(t, "ConfigMap", manifest.Kind)
	assert.Contains(t, manifest.Data.Input, "Name: my-release")
	assert.Contains(t, manifest.Data.Input, "replicas: 2")

	assert.Contains(t, rendered["ts-chart/charts/subchart/ts/src/index.js"], `"name":"subchart"`)
}

func TestRunBundle(t *testing.T) {
	skipWithoutShell(t)

	t.Setenv("NELM_TEST_CANARY", "leaked")

	nodePath := writeFakeNode(t, filepath.Join(t.TempDir(), "node"))
	renderDir := t.TempDir()

	require.NoError(t, ts.RunBundle(context.Background(), nodePath, renderDir, []byte("console.log('bundle');\n")))

	resolvedRenderDir := resolvePath(t, renderDir)
	inputPath := filepath.Join(resolvedRenderDir, "input.yaml")
	outputPath := filepath.Join(resolvedRenderDir, "output.yaml")

	run := readFakeNodeRun(t, nodePath)
	assert.Equal(t, []string{
		"--permission",
		"--no-experimental-sqlite",
		"--freeze-flags-after-init",
		"--allow-fs-read=" + inputPath,
		"--allow-fs-write=" + outputPath,
		"--input-type=module",
		"-",
		"--input-file=" + inputPath,
		"--output-file=" + outputPath,
	}, run.args)
	assert.Equal(t, "console.log('bundle');\n", run.stdin, "the bundle must be passed through stdin")
	assert.Equal(t, resolvedRenderDir, run.dir)
	assert.NotContains(t, run.env, "NELM_TEST_CANARY", "the environment must not be passed")
	assert.NotContains(t, run.env, "HOME=", "the environment must not be passed")
}

func TestRunBundleOutput(t *testing.T) {
	skipWithoutShell(t)

	t.Run("logs stdout as debug and stderr as warnings when the chart succeeds", func(t *testing.T) {
		logger := recordLogs(t)

		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte("#!/bin/sh\nprintf 'out 100%%\\n'\nprintf 'deprecated\\n' >&2\n"), 0o755))

		require.NoError(t, ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte("")))

		assert.Equal(t, []logEntry{{level: "debug", message: "out 100%"}, {level: "warn", message: "deprecated"}}, logger.entries())
	})

	t.Run("logs stderr as errors when the chart fails", func(t *testing.T) {
		logger := recordLogs(t)

		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte("#!/bin/sh\nprintf 'TypeError: boom\\n' >&2\nexit 1\n"), 0o755))

		err := ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 1")
		assert.Equal(t, []logEntry{{level: "error", message: "TypeError: boom"}}, logger.entries())
	})

	// The fake Node.js gets no PATH, so it is given the absolute paths of the tools.
	ddPath, err := exec.LookPath("dd")
	require.NoError(t, err)

	sleepPath, err := exec.LookPath("sleep")
	require.NoError(t, err)

	for name, redirect := range map[string]string{"stdout": " 2>/dev/null", "stderr": " >&2 2>/dev/null"} {
		t.Run("stops the chart writing too much to "+name, func(t *testing.T) {
			recordLogs(t)

			nodePath := filepath.Join(t.TempDir(), "node")
			require.NoError(t, os.WriteFile(nodePath, []byte("#!/bin/sh\n"+ddPath+" if=/dev/zero bs=1048576 count=20"+redirect+"\nexec "+sleepPath+" 60\n"), 0o755))

			start := time.Now()
			err := ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte(""))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the chart wrote more than 10 MiB to "+name)
			assert.Less(t, time.Since(start), 30*time.Second, "the chart must be stopped")
		})
	}
}

func TestRunBundleRejectsWildcardInPaths(t *testing.T) {
	skipWithoutShell(t)

	nodePath := writeFakeNode(t, filepath.Join(t.TempDir(), "node"))
	renderDir := filepath.Join(t.TempDir(), "my*chart")
	require.NoError(t, os.MkdirAll(renderDir, 0o755))

	err := ts.RunBundle(context.Background(), nodePath, renderDir, []byte(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `contains "*"`)
	assert.NoFileExists(t, nodePath+".args", "Node.js must not be run")
}

func TestRunBundleSandbox(t *testing.T) {
	nodePath := requireTestNode(t)

	t.Setenv("NELM_TEST_CANARY", "leaked")

	secretPath := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("secret"), 0o644))

	runProbe := func(t *testing.T, probe string) string {
		t.Helper()

		renderDir := t.TempDir()
		bundle := `import { writeFile } from 'node:fs/promises';
import { parseArgs } from 'node:util';
const { values } = parseArgs({ args: process.argv.slice(2), options: { 'input-file': { type: 'string' }, 'output-file': { type: 'string' } } });
let result;
try { result = String(await (async () => { ` + probe + ` })()); } catch (error) { result = 'denied: ' + error.code; }
await writeFile(values['output-file'], JSON.stringify({ result }));
`

		require.NoError(t, os.WriteFile(filepath.Join(renderDir, "input.yaml"), []byte("{}"), 0o644))

		require.NoError(t, ts.RunBundle(context.Background(), nodePath, renderDir, []byte(bundle)))

		entries, err := os.ReadDir(renderDir)
		require.NoError(t, err)
		assert.Len(t, entries, 2, "nothing but the input and output files must be in the render dir")

		output, err := os.ReadFile(filepath.Join(renderDir, "output.yaml"))
		require.NoError(t, err)

		var result struct {
			Result string `json:"result"`
		}

		require.NoError(t, yaml.Unmarshal(output, &result))

		return result.Result
	}

	t.Run("passes exactly the SDK arguments after the stdin marker", func(t *testing.T) {
		assert.Equal(t, `["-","--input-file","--output-file"]`, runProbe(t, `return JSON.stringify(process.argv.slice(1).map((arg) => arg.split('=')[0]));`))
	})

	t.Run("reads the input file", func(t *testing.T) {
		assert.Equal(t, "{}", runProbe(t, `const { readFile } = await import('node:fs/promises'); return await readFile(values['input-file'], 'utf8');`))
	})

	t.Run("cannot read other files", func(t *testing.T) {
		assert.Equal(t, "denied: ERR_ACCESS_DENIED", runProbe(t, `const { readFile } = await import('node:fs/promises'); return await readFile(`+jsString(secretPath)+`, 'utf8');`))
	})

	t.Run("cannot write other files", func(t *testing.T) {
		assert.Equal(t, "denied: ERR_ACCESS_DENIED", runProbe(t, `const { writeFile } = await import('node:fs/promises'); await writeFile(`+jsString(secretPath)+`, 'x');`))
	})

	t.Run("cannot spawn processes", func(t *testing.T) {
		assert.Equal(t, "denied: ERR_ACCESS_DENIED", runProbe(t, `const { execSync } = await import('node:child_process'); return execSync('id');`))
	})

	t.Run("cannot reach the network", func(t *testing.T) {
		assert.Equal(t, "denied: ERR_ACCESS_DENIED", runProbe(t, `const net = await import('node:net'); return await new Promise((resolve, reject) => { const socket = net.connect(80, '127.0.0.1'); socket.on('error', reject); socket.on('connect', () => resolve('connected')); });`))
	})

	t.Run("gets no environment", func(t *testing.T) {
		assert.NotContains(t, runProbe(t, `return Object.keys(process.env).join(',');`), "NELM_TEST_CANARY")
	})

	t.Run("cannot write files through node:sqlite", func(t *testing.T) {
		databasePath := filepath.Join(t.TempDir(), "escape.db")

		assert.Equal(t, "denied: ERR_UNKNOWN_BUILTIN_MODULE", runProbe(t, `const { DatabaseSync } = await import('node:sqlite'); new DatabaseSync(`+jsString(databasePath)+`).exec('create table t (x)'); return 'written';`))
		assert.NoFileExists(t, databasePath)
	})

	for name, flags := range map[string]func(dir string) []string{
		"redirected code traces": func(dir string) []string {
			return []string{"--redirect-code-traces-to=" + filepath.Join(dir, "traces.txt"), "--redirect-code-traces", "--print-opt-code"}
		},
		"turbofan traces": func(dir string) []string {
			return []string{"--trace-turbo", "--trace-turbo-path=" + dir}
		},
	} {
		t.Run("cannot write files through V8 flags for "+name, func(t *testing.T) {
			targetDir := t.TempDir()
			renderDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(renderDir, "input.yaml"), []byte("{}"), 0o644))

			var setFlags strings.Builder
			for _, flag := range flags(targetDir) {
				setFlags.WriteString("v8.setFlagsFromString(" + jsString(flag) + ");\n")
			}

			bundle := "import v8 from 'node:v8';\n" + setFlags.String() + `function hot(x) { let s = 0; for (let i = 0; i < 100; i++) s += (x * i) % 7; return s; }
let total = 0;
for (let i = 0; i < 300000; i++) total += hot(i);
`

			recordLogs(t)
			require.Error(t, ts.RunBundle(context.Background(), nodePath, renderDir, []byte(bundle)), "changing V8 flags must stop the chart")

			entries, err := os.ReadDir(targetDir)
			require.NoError(t, err)
			assert.Empty(t, entries)

			entries, err = os.ReadDir(renderDir)
			require.NoError(t, err)
			assert.Len(t, entries, 1, "nothing but the input file must be in the render dir")
		})
	}
}

func TestRunBundleWithCJSDependency(t *testing.T) {
	nodePath := requireTestNode(t)

	tsDir, err := filepath.Abs(filepath.Join("testdata", "fixtures", "cjs-dependency", "ts"))
	require.NoError(t, err)

	bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
	require.NoError(t, err)

	renderDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(renderDir, "input.yaml"), []byte("{}"), 0o644))

	require.NoError(t, ts.RunBundle(context.Background(), nodePath, renderDir, bundle),
		"CommonJS dependencies must be able to require builtins and use __dirname and __filename")

	output, err := os.ReadFile(filepath.Join(renderDir, "output.yaml"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"dirname":"string","filename":"string","joined":"a/b","platform":"string"}`, string(output))
}
