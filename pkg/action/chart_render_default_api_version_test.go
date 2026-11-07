package action

import (
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestChartRender_DefaultChartAPIVersionWithChartLock(t *testing.T) {
	baseDir := t.TempDir()
	writeChartPackTestFiles(t, baseDir, map[string]string{
		"mychart/Chart.yaml":        "name: mychart\nversion: 1.0.0\ndependencies:\n  - name: sub\n    version: 0.1.0\n    repository: file://../sub\n",
		"mychart/Chart.lock":        "dependencies:\n  - name: sub\n    version: 0.1.0\n    repository: file://../sub\ndigest: sha256:0\ngenerated: \"2024-01-01T00:00:00Z\"\n",
		"mychart/values.yaml":       "sub:\n  key: from-parent\n",
		"mychart/templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: root\n",
		"sub/Chart.yaml":            "apiVersion: v2\nname: sub\nversion: 0.1.0\n",
		"sub/templates/cm.yaml":     "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: sub\ndata:\n  key: {{ .Values.key }}\n",
	})

	rendered, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
		Chart:                  filepath.Join(baseDir, "mychart"),
		DefaultChartAPIVersion: v2chart.APIVersionV2,
		OutputNoPrint:          true,
		TempDirPath:            t.TempDir(),
	})
	require.NoError(t, err)

	data := lo.SliceToMap(rendered.Resources, func(res *spec.ResourceSpec) (string, interface{}) {
		return res.Name, res.Unstruct.Object["data"]
	})
	assert.Equal(t, map[string]interface{}{
		"root": nil,
		"sub":  map[string]interface{}{"key": "from-parent"},
	}, data)
}
