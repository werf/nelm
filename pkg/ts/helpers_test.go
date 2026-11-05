package ts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/ts"
	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

const fakeNodeScript = `#!/bin/sh
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

type fakeNodeRun struct {
	args  []string
	dir   string
	env   string
	stdin string
}

func gzipPayload(t *testing.T, payload []byte) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer

	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return buf.Bytes(), sha256Hex(payload)
}

// populateDownloadCache puts a fake Node.js binary and npm where the downloaded release is cached,
// so that they are used without downloading anything. It returns the cache directory.
func populateDownloadCache(t *testing.T) string {
	t.Helper()

	if embeddedNodeEnabled {
		t.Skip("the embedded Node.js takes priority over the download cache")
	}

	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	link, err := nodelock.ArchiveURL(runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)

	cacheDir, err := ts.GetNodeFolder(link)
	require.NoError(t, err)

	writeFakeNode(t, filepath.Join(cacheDir, nodelock.BinaryName(runtime.GOOS)))

	npmCLI := filepath.Join(cacheDir, "npm", "bin", "npm-cli.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(npmCLI), 0o755))
	require.NoError(t, os.WriteFile(npmCLI, []byte("// npm"), 0o644))

	return cacheDir
}

func tarGzNPM(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	fileSHA256s := map[string]string{}

	for name, content := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}))
		_, err := tarWriter.Write([]byte(content))
		require.NoError(t, err)

		fileSHA256s[name] = sha256Hex([]byte(content))
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzWriter.Close())

	return buf.Bytes(), nodelock.TreeSHA256(fileSHA256s)
}

func copyTestChart(t *testing.T) string {
	t.Helper()

	chartDir := filepath.Join(t.TempDir(), "ts-chart")
	require.NoError(t, os.CopyFS(chartDir, os.DirFS(filepath.Join("testdata", "chart"))))

	return chartDir
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func skipWithoutShell(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the fake Node.js binary is a shell script")
	}
}

// writeFakeNode writes a Node.js stand-in that records the arguments, environment, working
// directory and stdin it was run with next to itself, and writes a manifest to the file given in
// --output-file, if any.
func writeFakeNode(t *testing.T, path string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(fakeNodeScript), 0o755))

	return path
}
