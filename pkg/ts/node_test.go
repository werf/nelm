package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
)

func TestFindNode(t *testing.T) {
	const probeArgs = "'nelm-node-probe:' + JSON.stringify({ execPath: process.execPath, version: process.versions.node })"

	probeOutput := func(execPath, version string) string {
		return `printf '%s\n' 'nelm-node-probe:{"execPath":"` + execPath + `","version":"` + version + `"}'` + "\n"
	}

	t.Run("takes the real path and the version from node in PATH", func(t *testing.T) {
		binDir := stubPath(t)
		t.Setenv("NELM_TEST_CANARY", "inherited")
		writeStub(t, binDir, "node", probeOutput("/opt/node versions/26.13.0/bin/node", "26.13.0"))

		execPath, err := ts.FindNode(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "/opt/node versions/26.13.0/bin/node", execPath)

		run := readStubRun(t, binDir, "node")
		assert.Equal(t, []string{"-p", probeArgs}, run.args)
		assert.Equal(t, "inherited", run.canary, "the probe must run with the environment of nelm")
	})

	t.Run("ignores what preloaded modules print", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "node", "printf 'hello from preload\\n/not/node 1.0.0\\n'\n"+probeOutput("/opt/node/bin/node", "26.13.0")+"printf 'bye from preload\\n'\n")

		execPath, err := ts.FindNode(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "/opt/node/bin/node", execPath)
	})

	t.Run("prefers the node binary path over node in PATH", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "node", probeOutput("/usr/bin/node", "26.13.0"))

		explicitDir := t.TempDir()
		writeStub(t, explicitDir, "my-node", probeOutput("/opt/node/bin/node", "27.1.0"))

		execPath, err := ts.FindNode(context.Background(), filepath.Join(explicitDir, "my-node"))
		require.NoError(t, err)
		assert.Equal(t, "/opt/node/bin/node", execPath)
		requireStubNotRun(t, binDir, "node")
	})

	t.Run("accepts the first Node.js 26", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "node", probeOutput("/usr/bin/node", "26.0.0"))

		execPath, err := ts.FindNode(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/node", execPath)
	})

	for name, test := range map[string]struct {
		body    string
		wantErr []string
	}{
		"fails with Node.js 24": {
			body:    probeOutput("/usr/bin/node", "24.13.0"),
			wantErr: []string{"Node.js >= 26 is required to render TypeScript charts, put it in PATH or set the path to the Node.js binary: found Node.js 24.13.0 at /usr/bin/node"},
		},
		"fails with Node.js 25": {
			body:    probeOutput("/usr/bin/node", "25.9.0"),
			wantErr: []string{"Node.js >= 26 is required", "found Node.js 25.9.0 at /usr/bin/node"},
		},
		"fails when node does not run": {
			body:    "printf 'node: something went wrong\\n' >&2\nexit 3\n",
			wantErr: []string{"Node.js >= 26 is required", "exit status 3", "node: something went wrong"},
		},
		"suggests installing libatomic when node misses it": {
			body: "printf 'node: error while loading shared libraries: libatomic.so.1: cannot open shared object file: No such file or directory\\n' >&2\nexit 127\n",
			wantErr: []string{
				"libatomic.so.1: cannot open shared object file",
				"Install the libatomic1 package (Debian/Ubuntu) or libatomic (RHEL/Fedora).",
			},
		},
		"fails on unexpected output": {
			body:    "printf 'v26.13.0\\n'\n",
			wantErr: []string{`no line starting with "nelm-node-probe:" in the output: "v26.13.0\n"`},
		},
		"fails on an unparsable version": {
			body:    probeOutput("/usr/bin/node", "nightly"),
			wantErr: []string{`parse major version of "nightly"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			writeStub(t, binDir, "node", test.body)

			_, err := ts.FindNode(context.Background(), "")
			require.Error(t, err)

			for _, want := range test.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}

	t.Run("does not suggest a package for other failures", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "node", "exit 1\n")

		_, err := ts.FindNode(context.Background(), "")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "Install the")
	})

	t.Run("fails without node in PATH", func(t *testing.T) {
		stubPath(t)

		_, err := ts.FindNode(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Node.js >= 26 is required to render TypeScript charts, put it in PATH or set the path to the Node.js binary: no node in PATH")
	})

	t.Run("fails with a missing node binary path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "node")

		_, err := ts.FindNode(context.Background(), missing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "run "+missing)
		assert.NoFileExists(t, missing)
	})

	t.Run("finds a real Node.js", func(t *testing.T) {
		nodePath := requireTestNode(t)

		execPath, err := ts.FindNode(context.Background(), nodePath)
		require.NoError(t, err)

		info, err := os.Stat(execPath)
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(execPath))
		assert.False(t, info.IsDir())
	})
}
