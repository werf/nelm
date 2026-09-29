//go:build ai_tests

package chart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	invalidAnnotationManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: ok
  annotations:
    key:
      nested: value
`
	invalidManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: broken
data: [unclosed
`
	invalidNameManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: [broken]
`
	validManifestAI = `apiVersion: v1
kind: ConfigMap
metadata:
  name: ok
data:
  key: value
`
)

func TestAI_RenderedTemplatesToResourceSpecs_InvalidAnnotations_Rejected(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidAnnotationManifestAI}

	_, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse YAML resource #1")
}

func TestAI_RenderedTemplatesToResourceSpecs_NonScalarName_Accepted(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidNameManifestAI}

	resources, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Empty(t, resources[0].Name)
}

func TestAI_RenderedTemplatesToResourceSpecs_DuplicateMapKeys_Accepted(t *testing.T) {
	templates := map[string]string{"templates/dup.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: dup\ndata:\n  key: first\n  key: second\n"}

	resources, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.NoError(t, err)
	require.Len(t, resources, 1)
}

func TestAI_RenderedTemplatesToResourceSpecs_InvalidAnnotationsWithDropInvalidAnnotationsAndLabels_Stripped(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidAnnotationManifestAI}

	resources, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{
		DropInvalidAnnotationsAndLabels: true,
	})

	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Empty(t, resources[0].Annotations)
}

func TestAI_RenderedTemplatesToResourceSpecs_InvalidSecondDocument_ReportsItsIndex(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": validManifestAI + "---\n" + invalidManifestAI}

	_, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `parse YAML resource #2 for "templates/broken.yaml"`)
}

func TestAI_RenderedTemplatesToResourceSpecs_InvalidYAML_ReportsParseErrorWithPosition(t *testing.T) {
	templates := map[string]string{"templates/broken.yaml": invalidManifestAI}

	_, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `parse YAML resource #1 for "templates/broken.yaml"`)
	assert.Contains(t, err.Error(), "sequence end token ']' not found")
}

func TestAI_RenderedTemplatesToResourceSpecs_ValidManifest_Rendered(t *testing.T) {
	templates := map[string]string{"templates/ok.yaml": validManifestAI}

	resources, err := renderedTemplatesToResourceSpecs(context.Background(), templates, "ns", RenderChartOptions{})

	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "ok", resources[0].Name)
	assert.Equal(t, map[string]interface{}{"key": "value"}, resources[0].Unstruct.Object["data"])
}
