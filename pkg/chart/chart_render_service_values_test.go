package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderChart_WerfServiceValuesInSubcharts(t *testing.T) {
	const (
		imageSchema       = `{"type":"object","required":["image"],"properties":{"image":{"type":"object","required":["repository"],"properties":{"repository":{"type":"string"},"tag":{"type":"string"}}}}}`
		strictImageSchema = `{"type":"object","additionalProperties":false,"required":["image"],"properties":{"global":{"type":"object"},"image":{"type":"object","additionalProperties":false,"required":["repository"],"properties":{"repository":{"type":"string"},"tag":{"type":"string"}}}}}`
		subsubCondition   = "  condition: subsub.enabled\n"
		subsubDisabled    = "subsub:\n  enabled: false\n"
	)

	tests := []struct {
		name       string
		chart      map[string]string
		env        string
		values     string
		wantCharts []string
		wantValues map[string]interface{}
	}{
		{
			name: "global.werf.images exported from root chart to nested subchart satisfy its schema",
			chart: mergeTestFiles(
				werfServiceValuesTestChart("  export-values:\n  - parent: global.werf.images.app.repository\n    child: subsub.image.repository\n  - parent: global.werf.images.app.tag\n    child: subsub.image.tag\n", "", ""),
				map[string]string{"charts/sub/charts/subsub/values.schema.json": imageSchema},
			),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.image.repository": "NAMESPACE/NAME", "sub.subsub.image.tag": "TAG"},
		},
		{
			name: "global.werf.images exported from root chart to nested subchart satisfy its schema with additionalProperties false",
			chart: mergeTestFiles(
				werfServiceValuesTestChart("  export-values:\n  - parent: global.werf.images.app.repository\n    child: subsub.image.repository\n  - parent: global.werf.images.app.tag\n    child: subsub.image.tag\n", "", ""),
				map[string]string{"charts/sub/charts/subsub/values.schema.json": strictImageSchema},
			),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.image.repository": "NAMESPACE/NAME", "sub.subsub.image.tag": "TAG"},
		},
		{
			name: "legacy werf.image exported from root chart to nested subchart",
			chart: mergeTestFiles(
				werfServiceValuesTestChart("  export-values:\n  - parent: werf.image.app\n    child: subsub.image.repository\n", "", ""),
				map[string]string{"charts/sub/charts/subsub/values.schema.json": imageSchema},
			),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.image.repository": "REPO:TAG"},
		},
		{
			name: "legacy werf.image exported from root chart through subchart to nested subchart overrides subchart default",
			chart: mergeTestFiles(
				werfServiceValuesTestChart(
					"  export-values:\n  - parent: werf.image.app\n    child: appImage\n",
					"  export-values:\n  - parent: appImage\n    child: image.repository\n",
					"appImage: default/app:latest\n",
				),
				map[string]string{"charts/sub/charts/subsub/values.schema.json": imageSchema},
			),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.appImage": "REPO:TAG", "sub.subsub.image.repository": "REPO:TAG"},
		},
		{
			name:       "nested subchart disabled in subchart values stays disabled without export",
			chart:      werfServiceValuesTestChart("", subsubCondition, subsubDisabled),
			wantCharts: []string{"root", "sub"},
			wantValues: map[string]interface{}{"sub.subsub.enabled": false},
		},
		{
			name:       "global.werf.is_stub exported from root chart enables nested subchart disabled in subchart values",
			chart:      werfServiceValuesTestChart("  export-values:\n  - parent: global.werf.is_stub\n    child: subsub.enabled\n", subsubCondition, subsubDisabled),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.enabled": true},
		},
		{
			name:       "value from values file exported from root chart enables nested subchart disabled in subchart values",
			chart:      werfServiceValuesTestChart("  export-values:\n  - parent: flag\n    child: subsub.enabled\n", subsubCondition, subsubDisabled),
			values:     "flag: true",
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.enabled": true},
		},
		{
			name:       "non-bool werf.env exported from root chart leaves nested subchart condition ignored",
			chart:      werfServiceValuesTestChart("  export-values:\n  - parent: werf.env\n    child: subsub.enabled\n", subsubCondition, subsubDisabled),
			env:        "production",
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.enabled": "production"},
		},
		{
			name:       "condition on global.werf.is_stub takes precedence over nested subchart disabled in subchart values",
			chart:      werfServiceValuesTestChart("", "  condition: global.werf.is_stub,subsub.enabled\n", subsubDisabled),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.subsub.enabled": false},
		},
		{
			name: "global.werf.is_stub exported from root chart enables direct subchart disabled in root chart values",
			chart: mergeTestFiles(
				werfServiceValuesTestChart("  condition: sub.enabled\n  export-values:\n  - parent: global.werf.is_stub\n    child: enabled\n", "", ""),
				map[string]string{"values.yaml": "sub:\n  enabled: false\n"},
			),
			wantCharts: []string{"root", "sub", "subsub"},
			wantValues: map[string]interface{}{"sub.enabled": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := werfDataFlagsAsOptions(t, werfDataFlags{Values: tt.values, ExtraValues: werfStubServiceValues(tt.env)}, RenderChartOptions{})

			result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, tt.chart), opts)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCharts, renderedConfigMapNames(result))

			vals := renderedChartValues(t, result)
			for path, want := range tt.wantValues {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}
