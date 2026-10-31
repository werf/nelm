package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
)

func TestLoadFiles_RefusesWerfData(t *testing.T) {
	chartFiles := func(extra ...*BufferedFile) []*BufferedFile {
		return append([]*BufferedFile{
			{Name: "Chart.yaml", Data: []byte("apiVersion: v2\nname: app\nversion: 1.0.0\n")},
			{Name: "values.yaml", Data: []byte("key: value\n")},
		}, extra...)
	}

	tests := []struct {
		name    string
		files   []*BufferedFile
		wantErr string
	}{
		{
			name:  "chart without .werf-data",
			files: chartFiles(),
		},
		{
			name:    "chart with .werf-data apiVersion v2",
			files:   chartFiles(&BufferedFile{Name: ".werf-data/metadata.yaml", Data: []byte("apiVersion: v2\n")}),
			wantErr: `this chart has .werf-data/metadata.yaml with apiVersion "v2": it requires werf v3 or Nelm v2, or newer`,
		},
		{
			name:    "chart with .werf-data of a later version",
			files:   chartFiles(&BufferedFile{Name: ".werf-data/metadata.yaml", Data: []byte("apiVersion: v7\n")}),
			wantErr: "it requires werf v3 or Nelm v2, or newer",
		},
		{
			name: "dependency with .werf-data",
			files: chartFiles(
				&BufferedFile{Name: "charts/sub/Chart.yaml", Data: []byte("apiVersion: v2\nname: sub\nversion: 1.0.0\n")},
				&BufferedFile{Name: "charts/sub/.werf-data/metadata.yaml", Data: []byte("apiVersion: v2\n")},
			),
			wantErr: "it requires werf v3 or Nelm v2, or newer",
		},
		{
			name:  "unrelated file with the same base name",
			files: chartFiles(&BufferedFile{Name: "files/.werf-data/metadata.yaml", Data: []byte("apiVersion: v2\n")}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFiles(tt.files, helmopts.HelmOptions{})
			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}

			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestLoadDir_RefusesWerfDataIgnoredByHelmignore(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
	}{
		{
			name:  "no .werf-data",
			files: map[string]string{"templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"},
		},
		{
			name: ".werf-data outside of the chart root",
			files: map[string]string{
				"files/.werf-data/metadata.yaml": "apiVersion: v2\n",
			},
		},
		{
			name:    "root .werf-data",
			wantErr: true,
			files: map[string]string{
				".werf-data/metadata.yaml": "apiVersion: v2\n",
			},
		},
		{
			name:    "dependency .werf-data",
			wantErr: true,
			files: map[string]string{
				"charts/sub/Chart.yaml":              "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
				"charts/sub/.werf-data/metadata.yaml": "apiVersion: v2\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"Chart.yaml":  "apiVersion: v2\nname: app\nversion: 1.0.0\n",
				".helmignore": ".*\n",
			}
			for name, data := range tt.files {
				files[name] = data
			}

			for name, data := range files {
				path := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
			}

			_, err := LoadDir(dir, helmopts.HelmOptions{})
			if !tt.wantErr {
				require.NoError(t, err)

				return
			}

			assert.ErrorContains(t, err, "it requires werf v3 or Nelm v2, or newer")
		})
	}
}
