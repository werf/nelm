package ts_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/log"
)

const (
	fakeNodeScript = `#!/bin/sh
cat > "$0.stdin"
printf '%s\n' "$@" > "$0.args"
env > "$0.env"
pwd -P > "$0.pwd"
for arg; do
  case "$arg" in
    --output-file=*) printf 'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: fake\n' > "${arg#--output-file=}" ;;
  esac
done
`
	// stubScript records its arguments, working directory and $NELM_TEST_CANARY next to itself, and then
	// runs the body. It uses shell builtins only, as PATH in the tests holds nothing but the stubs.
	stubScript = `#!/bin/sh
printf '%s\n' "$@" > "$0.args"
pwd -P > "$0.pwd"
printf '%s' "$NELM_TEST_CANARY" > "$0.canary"
`
)

type logEntry struct {
	level   string
	message string
}

// recordingLogger records the debug, warning and error messages and passes the rest to log.Default.
type recordingLogger struct {
	log.Logger

	mu      sync.Mutex
	records []logEntry
}

func (l *recordingLogger) Debug(_ context.Context, format string, a ...interface{}) {
	l.record("debug", format, a)
}

func (l *recordingLogger) Error(_ context.Context, format string, a ...interface{}) {
	l.record("error", format, a)
}

func (l *recordingLogger) Warn(_ context.Context, format string, a ...interface{}) {
	l.record("warn", format, a)
}

func (l *recordingLogger) entries() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.records)
}

func (l *recordingLogger) record(level, format string, a []interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.records = append(l.records, logEntry{level: level, message: fmt.Sprintf(format, a...)})
}

type fakeNodeRun struct {
	args  []string
	dir   string
	env   string
	stdin string
}

type stubRun struct {
	args   []string
	canary string
	dir    string
}

// stubPath makes PATH hold nothing but a new directory and returns it.
func stubPath(t *testing.T) string {
	t.Helper()

	skipWithoutShell(t)

	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	return binDir
}

func copyTestChart(t *testing.T) string {
	t.Helper()

	chartDir := filepath.Join(t.TempDir(), "ts-chart")
	require.NoError(t, os.CopyFS(chartDir, os.DirFS(filepath.Join("testdata", "chart"))))

	return chartDir
}

func jsString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), "'", `\'`) + "'"
}

func loadChart(t *testing.T, chartDir string) helmchart.Accessor {
	t.Helper()

	ctx := common.ContextWithHelmOptions(context.Background(), common.HelmOptions{
		ChartLoadOpts: common.ChartLoadOptions{
			ChartType: common.LegacyChartTypeChart,
		},
	})

	loadedChart, err := loader.Load(ctx, chartDir)
	require.NoError(t, err)

	accessor, err := helmchart.NewAccessor(loadedChart)
	require.NoError(t, err)

	return accessor
}

func logMessages(entries []logEntry, level string) []string {
	return lo.FilterMap(entries, func(entry logEntry, _ int) (string, bool) {
		return entry.message, entry.level == level
	})
}

func readFakeNodeRun(t *testing.T, nodePath string) fakeNodeRun {
	t.Helper()

	args, err := os.ReadFile(nodePath + ".args")
	require.NoError(t, err, "the fake Node.js binary was not run")

	env, err := os.ReadFile(nodePath + ".env")
	require.NoError(t, err)

	dir, err := os.ReadFile(nodePath + ".pwd")
	require.NoError(t, err)

	stdin, err := os.ReadFile(nodePath + ".stdin")
	require.NoError(t, err)

	return fakeNodeRun{
		args:  strings.Split(strings.TrimSuffix(string(args), "\n"), "\n"),
		dir:   strings.TrimSpace(string(dir)),
		env:   string(env),
		stdin: string(stdin),
	}
}

func readStubRun(t *testing.T, binDir, name string) stubRun {
	t.Helper()

	path := filepath.Join(binDir, name)

	args, err := os.ReadFile(path + ".args")
	require.NoError(t, err, "%s was not run", name)

	dir, err := os.ReadFile(path + ".pwd")
	require.NoError(t, err)

	canary, err := os.ReadFile(path + ".canary")
	require.NoError(t, err)

	return stubRun{
		args:   strings.Split(strings.TrimSuffix(string(args), "\n"), "\n"),
		canary: string(canary),
		dir:    strings.TrimSpace(string(dir)),
	}
}

// recordLogs makes log.Default record messages until the test ends.
func recordLogs(t *testing.T) *recordingLogger {
	t.Helper()

	previous := log.Default
	logger := &recordingLogger{Logger: previous}
	log.Default = logger

	t.Cleanup(func() { log.Default = previous })

	return logger
}

func requireStubNotRun(t *testing.T, binDir, name string) {
	t.Helper()

	require.NoFileExists(t, filepath.Join(binDir, name+".args"), "%s must not be run", name)
}

// requireTestNode returns the Node.js to run the tests that need a real one with, taken from
// NELM_TEST_NODE_BINARY_PATH, and skips the test if it is not set.
func requireTestNode(t *testing.T) string {
	t.Helper()

	nodePath := os.Getenv("NELM_TEST_NODE_BINARY_PATH")
	if nodePath == "" {
		t.Skip("NELM_TEST_NODE_BINARY_PATH is not set")
	}

	return nodePath
}

func resolvePath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)

	return resolved
}

func skipWithoutShell(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stubs are shell scripts")
	}
}

func writeFakeNode(t *testing.T, path string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(fakeNodeScript), 0o755))

	return path
}

// writeFiles creates the files under dir, given their slash-separated paths and contents. A path
// ending with "/" is created as a directory.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))

		if strings.HasSuffix(name, "/") {
			require.NoError(t, os.MkdirAll(path, 0o755))

			continue
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// writeStub writes an executable named name into binDir that records how it was run and then runs
// body.
func writeStub(t *testing.T, binDir, name, body string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(stubScript+body), 0o755))
}
