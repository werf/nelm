package action

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
)

func TestReleaseInstall_TimeoutDuringReportReturnsAfterReport(t *testing.T) {
	api := newProgressPhasesAPI(t)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
		s.failConfigMaps = map[string]bool{"progress-resource": true}
	})

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: progress-resource\ndata:\n  key: value\n"), 0o600))

	const timeout = 2 * time.Second

	logs := &blockingLogWriter{
		blockOn:   "Failed operations",
		blockedCh: make(chan struct{}, 1),
		logs:      kdutil.NewConcurrent(&bytes.Buffer{}),
		releaseCh: make(chan struct{}),
	}

	reportBlockedCh := make(chan bool, 1)

	go func() {
		select {
		case <-logs.blockedCh:
			time.Sleep(timeout)

			reportBlockedCh <- true
		case <-time.After(5 * time.Second):
			reportBlockedCh <- false
		}

		close(logs.releaseCh)
	}()

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(logs, logs)), 10*time.Second)
	defer cancel()

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		Chart:                 chartPath,
		Timeout:               timeout,
		NoCreateNamespace:     true, LegacyNoReleaseLock: true, NoShowNotes: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
			ReleaseStorageDriver:      "configmap",
			ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		},
		TempDirPath: t.TempDir(),
	})
	require.True(t, <-reportBlockedCh, "the report must be printed before the timeout")
	require.ErrorContains(t, err, "failed release")
	assert.NotContains(t, err.Error(), "action timed out", "the release failed before the timeout, the timeout only hit while the report was printed")

	logsOnReturn := logs.String()
	assert.Contains(t, logsOnReturn, "Create: ConfigMap/progress-resource")

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, logsOnReturn, logs.String(), "nothing must be logged after the error is returned")
}
