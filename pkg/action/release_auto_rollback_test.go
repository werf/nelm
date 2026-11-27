package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseInstall_AutoRollbackAfterFailedApply(t *testing.T) {
	api := newProgressPhasesAPI(t)
	manifest := addReleaseConfigMap(t, api, "progress-resource", nil)
	addProgressPhaseRevision(t, api, 1, manifest)

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
		s.failConfigMaps = map[string]bool{"progress-resource": true}
		s.failedApplyPersists = true
	})

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(strings.Replace(manifest, `"key":"value"`, `"key":"changed"`, 1)), 0o600))

	ctx := newStuckRevisionContext(t)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = chartPath
	opts.AutoRollback = true

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.ErrorContains(t, err, `server-side apply resource "ConfigMap/progress-resource"`)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, ctx, api))

	api.state.RTransaction(func(s *progressPhasesAPIState) {
		require.Len(t, s.appliedConfigMaps, 1)
		assert.Equal(t, "progress-resource", s.appliedConfigMaps[0].Name)
		assert.Equal(t, map[string]string{"key": "value"}, s.configMaps["progress-resource"].Data)
	})
}
