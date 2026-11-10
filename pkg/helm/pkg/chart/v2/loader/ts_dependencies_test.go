package loader

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
)

func TestLoadFilesSkipsTSDependencies(t *testing.T) {
	files := []*archive.BufferedFile{
		{Name: "Chart.yaml", Data: []byte("apiVersion: v2\nname: mychart\nversion: 1.0.0\n")},
		{Name: "ts/src/index.ts", Data: []byte("export {};\n")},
		{Name: "charts/sub/Chart.yaml", Data: []byte("apiVersion: v2\nname: sub\nversion: 1.0.0\n")},
	}

	for _, path := range []string{"ts/node_modules/dep/index.js", "ts/.yarn/install-state.gz", "ts/.pnp.cjs", "ts/.pnp.loader.mjs"} {
		files = append(files,
			&archive.BufferedFile{Name: path, Data: []byte("x\n")},
			&archive.BufferedFile{Name: "charts/sub/" + path, Data: []byte("x\n")},
		)
	}

	files = append(files, &archive.BufferedFile{Name: "charts/sub/ts/node_modules/dep/dep.prov", Data: []byte("provenance\n")})

	loaded, err := LoadFiles(context.Background(), files)
	require.NoError(t, err)
	require.Len(t, loaded.Dependencies(), 1)

	for _, file := range append(loaded.Files, loaded.Dependencies()[0].Files...) {
		for _, part := range []string{"node_modules", ".yarn", ".pnp."} {
			assert.False(t, strings.Contains(file.Name, part), "file %s must be skipped", file.Name)
		}
	}
}
