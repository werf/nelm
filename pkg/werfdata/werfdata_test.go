package werfdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
)

func TestIsWerfDataFile(t *testing.T) {
	assert.True(t, IsWerfDataFile(".werf-data"))
	assert.True(t, IsWerfDataFile(".werf-data/metadata.yaml"))
	assert.False(t, IsWerfDataFile(".werf-database/x"))
	assert.False(t, IsWerfDataFile("charts/sub/.werf-data/metadata.yaml"))
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:    "no files",
			files:   map[string]string{},
			wantErr: ".werf-data/metadata.yaml not found",
		},
		{
			name:    "no metadata",
			files:   map[string]string{".werf-data/values.yaml": "a: 1"},
			wantErr: ".werf-data/metadata.yaml not found",
		},
		{
			name:    "unknown apiVersion",
			files:   map[string]string{".werf-data/metadata.yaml": "apiVersion: v3"},
			wantErr: `unsupported .werf-data/metadata.yaml apiVersion "v3"`,
		},
		{
			name:    "missing apiVersion",
			files:   map[string]string{".werf-data/metadata.yaml": "values: {}"},
			wantErr: `unsupported .werf-data/metadata.yaml apiVersion ""`,
		},
		{
			name:    "unknown field",
			files:   map[string]string{".werf-data/metadata.yaml": "apiVersion: v2\nunknown: 1"},
			wantErr: "parse .werf-data/metadata.yaml",
		},
		{
			name:    "values file without flag",
			files:   map[string]string{".werf-data/metadata.yaml": "apiVersion: v2", ".werf-data/values.yaml": "a: 1"},
			wantErr: "values: false does not match the presence of values.yaml",
		},
		{
			name:    "flag without secret values file",
			files:   map[string]string{".werf-data/metadata.yaml": "apiVersion: v2\nvalues:\n  secretValues: true"},
			wantErr: "secretValues: true does not match the presence of secret-values.yaml",
		},
		{
			name: "set-file outside of set-files",
			files: map[string]string{
				".werf-data/metadata.yaml": "apiVersion: v2\nvalues:\n  valuesSetFile:\n  - value: a=f\n    contents: {f: ../../etc/passwd}",
			},
			wantErr: "points outside of set-files",
		},
		{
			name: "missing set-file contents",
			files: map[string]string{
				".werf-data/metadata.yaml": "apiVersion: v2\nvalues:\n  valuesSetFile:\n  - value: a=f\n    contents: {f: set-files/0}",
			},
			wantErr: ".werf-data/set-files/0 not found",
		},
		{
			name:    "patches outside of patches",
			files:   map[string]string{".werf-data/metadata.yaml": "apiVersion: v2\npatches: [values.yaml]"},
			wantErr: "patches file points outside of patches",
		},
		{
			name:    "file outside of .werf-data",
			files:   map[string]string{"values.yaml": "a: 1"},
			wantErr: `file "values.yaml" is not in .werf-data`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []*common.File
			for name, data := range tt.files {
				files = append(files, &common.File{Name: name, Data: []byte(data)})
			}

			_, err := Parse(files)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestParse_RoundTrip(t *testing.T) {
	data := &Data{
		Metadata: Metadata{
			Values: ValuesMetadata{
				ValuesSet:     []string{"replicas=3"},
				ValuesSetFile: []SetFileRecord{{Value: "cfg=./nginx.conf", Contents: map[string]string{"./nginx.conf": "set-files/0"}}},
			},
			Patches:          []string{"patches/0"},
			Release:          &ReleaseMetadata{Name: "app", Namespace: "ns", Env: "production"},
			Images:           []string{"backend"},
			ExtraAnnotations: map[string]string{"team": "payments"},
		},
		Values:       map[string]any{"key": "values"},
		SecretValues: []byte("secret: 1000abc\n"),
		ExtraValues:  map[string]any{"werf": map[string]any{"repo": "registry.example.com/app"}},
		Files: map[string][]byte{
			"set-files/0": []byte("nginx config"),
			"patches/0":   []byte("renderPatches: []\n"),
		},
	}

	files, err := data.ChartFiles()
	require.NoError(t, err)
	assert.Equal(t, []string{
		".werf-data/extra-values.yaml",
		".werf-data/metadata.yaml",
		".werf-data/patches/0",
		".werf-data/secret-values.yaml",
		".werf-data/set-files/0",
		".werf-data/values.yaml",
	}, fileNames(files))

	parsed, err := Parse(files)
	require.NoError(t, err)

	data.Metadata.APIVersion = APIVersionV2
	data.Metadata.Values.Values = true
	data.Metadata.Values.SecretValues = true
	data.Metadata.Values.ExtraValues = true
	assert.Equal(t, data, parsed)

	contents, err := parsed.SetFileContents(parsed.Metadata.Values.ValuesSetFile[0])
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"./nginx.conf": "nginx config"}, contents)

	patches, err := parsed.PatchesFiles()
	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("renderPatches: []\n")}, patches)
}

func fileNames(files []*common.File) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}

	return names
}
