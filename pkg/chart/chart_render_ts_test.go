package chart

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestRenderChart_TypeScript(t *testing.T) {
	nodePath := requireTestNode(t)

	chartPath, err := filepath.Abs(filepath.Join("testdata", "ts-chart"))
	require.NoError(t, err)

	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	result, err := RenderChart(ctx, chartPath, "rel", "ns", 1, common.DeployTypeInitial, nil, nil, RenderChartOptions{
		NodeBinaryPath: nodePath,
		TempDirPath:    t.TempDir(),
	})
	require.NoError(t, err)

	configMap, found := lo.Find(result.ResourceSpecs, func(res *spec.ResourceSpec) bool {
		return res.GroupVersionKind.Kind == "ConfigMap" && res.Name == "ts-chart"
	})
	require.True(t, found, "the manifest rendered by TypeScript must be among the resources")

	assert.Equal(t, "ts-chart/ts/src/index.js", configMap.FilePath)

	input, found, err := unstructured.NestedString(configMap.Unstruct.Object, "data", "input")
	require.NoError(t, err)
	require.True(t, found)
	assert.Contains(t, input, "Name: rel")
	assert.Contains(t, input, "replicas: 2")
}
