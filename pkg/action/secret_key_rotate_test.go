package action

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretKeyRotate_RestoresEnv(t *testing.T) {
	t.Setenv("WERF_SECRET_KEY", "previous")
	t.Setenv("WERF_OLD_SECRET_KEY", "")
	require.NoError(t, os.Unsetenv("WERF_OLD_SECRET_KEY"))

	chartDir := newSecretValuesTypesChart(t)
	encryptSecretValuesTypesFile(t, filepath.Join(chartDir, "secret-values.yaml"))

	require.NoError(t, SecretKeyRotate(context.Background(), SecretKeyRotateOptions{
		ChartDirPath:  chartDir,
		NewSecretKey:  "fedcba9876543210fedcba9876543210",
		OldSecretKey:  secretValuesTypesSecretKey,
		SecretWorkDir: chartDir,
		TempDirPath:   t.TempDir(),
	}))

	assert.Equal(t, "previous", os.Getenv("WERF_SECRET_KEY"))

	_, isOldSet := os.LookupEnv("WERF_OLD_SECRET_KEY")
	assert.False(t, isOldSet)
}
