package chart

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderChart_VendoredDependencyOfAnotherVersion_Rejected(t *testing.T) {
	tests := []struct {
		name        string
		chart       map[string]string
		expectedErr string
	}{
		{
			name: "locked",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.0\n    repository: https://example.invalid\n",
				"Chart.lock":            "dependencies:\n- name: dep\n  repository: https://example.invalid\n  version: 0.2.0\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
			},
			expectedErr: "charts/dep is version 0.1.0 but Chart.lock requires 0.2.0",
		},
		{
			name: "locked with other build metadata",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.0+build2\n    repository: https://example.invalid\n",
				"Chart.lock":            "dependencies:\n- name: dep\n  repository: https://example.invalid\n  version: 0.2.0+build2\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.2.0+build1\n",
			},
			expectedErr: "charts/dep is version 0.2.0+build1 but Chart.lock requires 0.2.0+build2",
		},
		{
			name: "stale archive next to the locked directory",
			chart: map[string]string{
				"Chart.yaml":                  "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.0+build2\n    repository: https://example.invalid\n",
				"Chart.lock":                  "dependencies:\n- name: dep\n  repository: https://example.invalid\n  version: 0.2.0+build2\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
				"charts/dep/Chart.yaml":       "apiVersion: v2\nname: dep\nversion: 0.2.0+build2\n",
				"charts/dep-0.2.0+build1.tgz": dependencyArchive(t, "dep", "0.2.0+build1"),
			},
			expectedErr: "charts/dep-0.2.0+build1.tgz is version 0.2.0+build1 but Chart.lock requires 0.2.0+build2: remove it",
		},
		{
			name: "vendored as both a directory and an archive",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.0\n    repository: https://example.invalid\n",
				"Chart.lock":            "dependencies:\n- name: dep\n  repository: https://example.invalid\n  version: 0.2.0\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.2.0\n",
				"charts/vendored.tgz":   dependencyArchive(t, "dep", "0.2.0"),
			},
			expectedErr: `dependency "dep" 0.2.0 is vendored more than once`,
		},
		{
			name: "unlocked",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.x\n    repository: https://example.invalid\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
			},
			expectedErr: "charts/dep is version 0.1.0 but Chart.yaml requires 0.2.x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, tt.chart), RenderChartOptions{})
			require.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

// dependencyArchive returns a chart archive with just Chart.yaml.
func dependencyArchive(t *testing.T, name, version string) string {
	t.Helper()

	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	chartYAML := fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", name, version)

	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name + "/Chart.yaml", Mode: 0o644, Size: int64(len(chartYAML))}))
	_, err := tarWriter.Write([]byte(chartYAML))
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.String()
}
