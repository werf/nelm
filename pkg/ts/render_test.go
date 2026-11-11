package ts_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/ts"
)

func TestRenderChartRemovesRenderDir(t *testing.T) {
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

			entries, err := os.ReadDir(filepath.Join(tempDir, "typescript-render"))
			require.NoError(t, err)
			assert.Empty(t, entries, "the render dir with the input, which has the values, must be removed")
		})
	}
}

func TestRenderChartWithNode(t *testing.T) {
	putTestNodeInPath(t)

	chartDir := copyTestChart(t)
	chart := loadChart(t, chartDir)

	rendered, err := ts.RenderChart(context.Background(), chart, chartcommon.Values{
		"Release": map[string]any{"Name": "my-release"},
		"Values":  map[string]any{"replicas": 2},
	}, false, chartDir, t.TempDir())
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
		"--no-deprecation",
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

	t.Run("logs stdout as debug and stderr as errors", func(t *testing.T) {
		logger := recordLogs(t)

		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte("#!/bin/sh\nprintf 'out 100%%\\n'\nprintf 'deprecated\\n' >&2\nprintf 'out 2\\n'\nprintf 'still deprecated\\n' >&2\n"), 0o755))

		require.NoError(t, ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte("")))

		// Stdout and stderr are read in parallel, so only the order within each of them is kept.
		entries := logger.entries()
		assert.ElementsMatch(t, []logEntry{
			{level: "debug", message: "out 100%"},
			{level: "debug", message: "out 2"},
			{level: "error", message: "error from node app: deprecated"},
			{level: "error", message: "error from node app: still deprecated"},
		}, entries)
		assert.Equal(t, []string{"out 100%", "out 2"}, logMessages(entries, "debug"))
		assert.Equal(t, []string{"error from node app: deprecated", "error from node app: still deprecated"}, logMessages(entries, "error"))
	})

	t.Run("fails with the exit status and logs all of a long error message", func(t *testing.T) {
		logger := recordLogs(t)

		var script strings.Builder
		script.WriteString("#!/bin/sh\n{\nprintf 'Error: Validation failed: [\\n'\n")

		for i := range 28 {
			fmt.Fprintf(&script, "printf '  { \"path\": [\"spec\", %d], \"message\": \"Required\" },\\n'\n", i)
		}

		script.WriteString("printf ']\\n'\n} >&2\nexit 3\n")

		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte(script.String()), 0o755))

		err := ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte(""))
		require.Error(t, err)
		assert.Equal(t, "wait process: exit status 3", err.Error())

		errors := logMessages(logger.entries(), "error")
		require.Len(t, errors, 30)
		assert.Equal(t, "error from node app: Error: Validation failed: [", errors[0])
		assert.Equal(t, `error from node app:   { "path": ["spec", 27], "message": "Required" },`, errors[28])
		assert.Equal(t, "error from node app: ]", errors[29])
	})

	// The fake Node.js gets no PATH, so it is given the absolute paths of the tools.
	sleepPath, err := exec.LookPath("sleep")
	require.NoError(t, err)

	catPath, err := exec.LookPath("cat")
	require.NoError(t, err)

	t.Run("truncates a long line and goes on reading", func(t *testing.T) {
		lines := runOutput(t, catPath, strings.Repeat("x", 1<<20)+"\nafter the long line\n")

		require.Len(t, lines, 2)
		assert.True(t, strings.HasPrefix(lines[0], strings.Repeat("x", 60<<10)))
		assert.Contains(t, lines[0], "[line truncated, ")
		assert.LessOrEqual(t, len(lines[0]), 64<<10)
		assert.Equal(t, "after the long line", lines[1])
	})

	t.Run("drops carriage returns at the ends of lines", func(t *testing.T) {
		assert.Equal(t, []string{"a", "b", "c\rd", "last"}, runOutput(t, catPath, "a\r\nb\r\nc\rd\nlast\r"))
	})

	t.Run("keeps empty lines of carriage returns alone", func(t *testing.T) {
		assert.Equal(t, []string{""}, runOutput(t, catPath, "\r"))
		assert.Equal(t, []string{"a", ""}, runOutput(t, catPath, "a\n\r"))
		assert.Equal(t, []string{"a", ""}, runOutput(t, catPath, "a\n\r\n"))
	})

	t.Run("keeps a line of exactly the limit whole", func(t *testing.T) {
		fullLine := strings.Repeat("x", 64<<10)

		assert.Equal(t, []string{fullLine, fullLine, "next"}, runOutput(t, catPath, fullLine+"\r\n"+fullLine+"\nnext\n"))
	})

	t.Run("does not cut a character when truncating a line", func(t *testing.T) {
		line := strings.Repeat("x", 65471) + "é" + strings.Repeat("y", 200)

		lines := runOutput(t, catPath, line+"\r\n")

		require.Len(t, lines, 1)
		assert.True(t, utf8.ValidString(lines[0]), "the truncated line must be valid UTF-8")
		assert.Equal(t, strings.Repeat("x", 65471)+fmt.Sprintf(" [line truncated, %d more bytes]", len(line)-65471), lines[0])
	})

	t.Run("keeps the last lines of a chart that exits right after writing them", func(t *testing.T) {
		const lineCount = 5000

		var lines strings.Builder
		for i := range lineCount {
			fmt.Fprintf(&lines, "line %05d of the chart output\n", i)
		}

		linesPath := filepath.Join(t.TempDir(), "lines")
		require.NoError(t, os.WriteFile(linesPath, []byte(lines.String()), 0o644))

		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte("#!/bin/sh\n"+catPath+" '"+linesPath+"' &\n"+catPath+" '"+linesPath+"' >&2\nwait\n"), 0o755))

		for range 50 {
			logger := recordLogs(t)

			require.NoError(t, ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte("")))

			entries := logger.entries()
			require.Len(t, logMessages(entries, "debug"), lineCount)
			require.Len(t, logMessages(entries, "error"), lineCount)
			assert.Equal(t, fmt.Sprintf("line %05d of the chart output", lineCount-1), logMessages(entries, "debug")[lineCount-1])
			assert.Equal(t, fmt.Sprintf("error from node app: line %05d of the chart output", lineCount-1), logMessages(entries, "error")[lineCount-1])
		}
	})

	t.Run("logs stderr while the chart runs", func(t *testing.T) {
		logger := recordLogs(t)

		releaseFile := filepath.Join(t.TempDir(), "release")
		nodePath := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(nodePath, []byte(`#!/bin/sh
printf 'still rendering\n' >&2
i=0
while [ ! -e '`+releaseFile+`' ] && [ $i -lt 100 ]; do `+sleepPath+` 0.1; i=$((i+1)); done
[ -e '`+releaseFile+`' ]
`), 0o755))

		done := make(chan error, 1)
		go func() { done <- ts.RunBundle(context.Background(), nodePath, t.TempDir(), []byte("")) }()

		logged := assert.Eventually(t, func() bool {
			return slices.Contains(logger.entries(), logEntry{level: "error", message: "error from node app: still rendering"})
		}, 5*time.Second, 10*time.Millisecond, "a stderr line must be logged before the chart exits")

		if logged {
			require.NoError(t, os.WriteFile(releaseFile, nil, 0o644))
		}

		assert.NoError(t, <-done)
	})
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

	t.Run("writes no deprecation warnings", func(t *testing.T) {
		logger := recordLogs(t)

		assert.Equal(t, "object", runProbe(t, `const { createRequire } = await import('node:module'); return typeof createRequire(import.meta.url)('punycode');`))
		assert.Empty(t, logMessages(logger.entries(), "error"))
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
