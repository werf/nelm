//go:build ai_tests

package chart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	invalidManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: broken
data: [unclosed
`
	validManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: ok
data:
  key: value
`
)

func TestAI_RenderedTemplatesToResourceSpecs_PrecheckDisabled_SkipsGoccyParse(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidManifestAI}

	_, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{
		NoManifestYAMLPrecheck: true,
	})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "parse YAML resource")
	assert.Contains(t, err.Error(), "decode resource")
}

func TestAI_RenderedTemplatesToResourceSpecs_PrecheckEnabled_ReportsParseYAMLError(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidManifestAI}

	_, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse YAML resource")
}

func TestAI_RenderedTemplatesToResourceSpecs_ValidManifest_SameResultBothModes(t *testing.T) {
	templates := map[string]string{"templates/ok.yaml": validManifestAI}

	withPrecheck, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})
	require.NoError(t, err)

	withoutPrecheck, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{
		NoManifestYAMLPrecheck: true,
	})
	require.NoError(t, err)

	require.Len(t, withPrecheck, 1)
	require.Len(t, withoutPrecheck, 1)
	assert.Equal(t, withPrecheck[0].Unstruct.Object, withoutPrecheck[0].Unstruct.Object)
}
