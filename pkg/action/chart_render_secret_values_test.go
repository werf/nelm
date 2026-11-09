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

func TestChartRender_SecretKey_RestoresEnv(t *testing.T) {
	t.Run("previously set", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", "previous")

		renderSecretValuesTypesChart(t)

		assert.Equal(t, "previous", os.Getenv("WERF_SECRET_KEY"))
	})

	t.Run("previously unset", func(t *testing.T) {
		t.Setenv("WERF_SECRET_KEY", "")
		require.NoError(t, os.Unsetenv("WERF_SECRET_KEY"))

		renderSecretValuesTypesChart(t)

		_, isSet := os.LookupEnv("WERF_SECRET_KEY")
		assert.False(t, isSet)
	})
}

func TestChartRender_SecretValues_KeepTypesForSchemaValidation(t *testing.T) {
	const wantValuesJSON = `{"bool":true,"int":123,"list":[1,"two"],"map":{"k":7},"number":1.5,"str":"456"}`

	t.Run("default secret-values.yaml", func(t *testing.T) {
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
