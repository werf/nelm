package action

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
)

func TestChartRender_SecretValues_KeepTypesForSchemaValidation(t *testing.T) {
	const wantValuesJSON = `{"bool":true,"int":123,"list":[1,"two"],"map":{"k":7},"number":1.5,"str":"456"}`

	t.Run("default secret-values.yaml", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", secretValuesTypesSecretKey)

		chartDir := newSecretValuesTypesChart(t)
		encryptSecretValuesTypesFile(t, filepath.Join(chartDir, "secret-values.yaml"))

		res, err := ChartRender(context.Background(), ChartRenderOptions{
			Chart:         chartDir,
			OutputNoPrint: true,
			SecretValuesOptions: common.SecretValuesOptions{
				SecretKey: secretValuesTypesSecretKey,
			},
			TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		assert.JSONEq(t, wantValuesJSON, renderedValuesJSON(t, res))
	})

	t.Run("custom secret values file", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", secretValuesTypesSecretKey)

		chartDir := newSecretValuesTypesChart(t)
		secretValuesPath := filepath.Join(t.TempDir(), "secret-values.yaml")
		encryptSecretValuesTypesFile(t, secretValuesPath)

		res, err := ChartRender(context.Background(), ChartRenderOptions{
			Chart:         chartDir,
			OutputNoPrint: true,
			SecretValuesOptions: common.SecretValuesOptions{
				SecretKey:         secretValuesTypesSecretKey,
				SecretValuesFiles: []string{secretValuesPath},
			},
			TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		assert.JSONEq(t, wantValuesJSON, renderedValuesJSON(t, res))
	})
}

func TestChartRender_SecretValues_WarnAboutUntypedLegacyValues(t *testing.T) {
	const warning = "Secret values file \"secret-values.yaml\" has values encrypted in an old format that does not store YAML types, so they are read as strings: int, bool, number, list[0], map.k, str. To make them numbers or booleans, re-enter them with `nelm chart secret values-file edit` (`werf helm secret values edit` in werf).\n"

	t.Run("legacy format", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", secretValuesTypesSecretKey)

		chartDir := newSecretValuesTypesChart(t)
		require.NoError(t, os.Remove(filepath.Join(chartDir, "values.schema.json")))
		require.NoError(t, os.CopyFS(chartDir, os.DirFS("testdata/secret-values-types/legacy")))

		ctx, logs := newLogCapturingContext(t)
		res, err := ChartRender(ctx, ChartRenderOptions{
			Chart:         chartDir,
			OutputNoPrint: true,
			SecretValuesOptions: common.SecretValuesOptions{
				SecretKey: secretValuesTypesSecretKey,
			},
			TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"bool":"true","int":"123","list":["1","two"],"map":{"k":"7"},"number":"1.5","str":"456"}`, renderedValuesJSON(t, res))
		assert.Equal(t, warning, logs.String())
	})

	t.Run("legacy format failing the values schema with service values", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", secretValuesTypesSecretKey)

		chartDir := newSecretValuesTypesChart(t)
		require.NoError(t, os.CopyFS(chartDir, os.DirFS("testdata/secret-values-types/legacy")))

		ctx, logs := newLogCapturingContext(t)
		_, err := ChartRender(ctx, ChartRenderOptions{
			Chart:             chartDir,
			LegacyExtraValues: map[string]interface{}{"werf": map[string]interface{}{"name": "project"}},
			OutputNoPrint:     true,
			SecretValuesOptions: common.SecretValuesOptions{
				SecretKey: secretValuesTypesSecretKey,
			},
			TempDirPath: t.TempDir(),
		})
		require.ErrorContains(t, err, "want integer")
		assert.Equal(t, warning, logs.String())
	})

	t.Run("current format", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", secretValuesTypesSecretKey)

		chartDir := newSecretValuesTypesChart(t)
		encryptSecretValuesTypesFile(t, filepath.Join(chartDir, "secret-values.yaml"))

		ctx, logs := newLogCapturingContext(t)
		_, err := ChartRender(ctx, ChartRenderOptions{
			Chart:         chartDir,
			OutputNoPrint: true,
			SecretValuesOptions: common.SecretValuesOptions{
				SecretKey: secretValuesTypesSecretKey,
			},
			TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		assert.Empty(t, logs.String())
	})
}
