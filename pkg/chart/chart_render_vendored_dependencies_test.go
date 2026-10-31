package chart

import (
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
