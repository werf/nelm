package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/ts"
)

// The tests below run a real Node.js, which they take from NELM_TEST_NODE_BINARY_PATH: the pinned
// release is not downloaded in unit tests.

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
}

func jsString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), "'", `\'`) + "'"
}

func requireTestNode(t *testing.T) string {
	t.Helper()

	nodePath := os.Getenv("NELM_TEST_NODE_BINARY_PATH")
	if nodePath == "" {
		t.Skip("NELM_TEST_NODE_BINARY_PATH is not set")
	}

	return nodePath
}
