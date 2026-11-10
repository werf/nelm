package loader

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
)

func TestLoadFilesSkipsTSDependencies(t *testing.T) {
	files := []*archive.BufferedFile{
		{Name: "Chart.yaml", Data: []byte("apiVersion: v2\nname: mychart\nversion: 1.0.0\n")},
		{Name: "ts/src/index.ts", Data: []byte("export {};\n")},
		{Name: "charts/sub/Chart.yaml", Data: []byte("apiVersion: v2\nname: sub\nversion: 1.0.0\n")},
	}

	skipped := []string{
		"ts/node_modules/dep/index.js",
		"ts/.yarn/install-state.gz",
		"ts/.yarn/build-state.yml",
		"ts/.yarn/cache/dep.zip",
		"ts/.yarn/unplugged/dep/index.js",
		"ts/.yarn/__virtual__/dep/0/index.js",
		"ts/.pnp.cjs",
		"ts/.pnp.loader.mjs",
	}
	kept := []string{"ts/.yarn/releases/yarn-4.18.1.cjs", "ts/.yarn/patches/dep.patch", "ts/.yarn/plugins/plugin.cjs"}

	for _, path := range append(slices.Clone(skipped), kept...) {
		files = append(files,
			&archive.BufferedFile{Name: path, Data: []byte("x\n")},
			&archive.BufferedFile{Name: "charts/sub/" + path, Data: []byte("x\n")},
		)
	}

	files = append(files, &archive.BufferedFile{Name: "charts/sub/ts/node_modules/dep/dep.prov", Data: []byte("provenance\n")})

	loaded, err := LoadFiles(context.Background(), files)
	require.NoError(t, err)
	require.Len(t, loaded.Dependencies(), 1)

	dep := loaded.Dependencies()[0]

	// The ts/ files of a subchart are runtime files.
	for _, chartFiles := range [][]*common.File{append(loaded.Files, loaded.RuntimeFiles...), append(dep.Files, dep.RuntimeFiles...)} {
		names := lo.Map(chartFiles, func(file *common.File, _ int) string { return file.Name })

		for _, path := range kept {
			assert.Contains(t, names, path)
		}

		for _, name := range names {
			assert.False(t, slices.Contains(skipped, name) || strings.Contains(name, "node_modules"), "file %s must be skipped", name)
		}
	}
}
