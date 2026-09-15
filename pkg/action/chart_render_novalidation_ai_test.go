//go:build ai_tests

package action

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAI_ChartRender_NoValuesSchemaValidation_SkipsValidation(t *testing.T) {
	dir := writeSchemaViolatingChartAI(t)

	_, errWithout := ChartRender(context.Background(), ChartRenderOptions{
		Chart:         dir,
		OutputNoPrint: true,
	})
	require.Error(t, errWithout, "render must fail when user values violate the schema and the flag is off")

	_, errWith := ChartRender(context.Background(), ChartRenderOptions{
		Chart:                    dir,
		OutputNoPrint:            true,
		NoValuesSchemaValidation: true,
	})
	assert.NoError(t, errWith, "render must succeed when values-schema validation is disabled")
}

func writeSchemaViolatingChartAI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("apiVersion: v2\nname: novalidation\nversion: 0.1.0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("foo: bar\nbadkey: oops\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "values.schema.json"), []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": { "foo": { "type": "string" } }
}`), 0o644))

	tmplDir := filepath.Join(dir, "templates")
	require.NoError(t, os.MkdirAll(tmplDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmplDir, "cm.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\ndata:\n  foo: {{ .Values.foo }}\n"), 0o644))

	return dir
}
