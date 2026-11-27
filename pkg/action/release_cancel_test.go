package action

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

const canceledResourceManifest = `apiVersion: v1
kind: ConfigMap
metadata:
  name: canceled-resource
  namespace: progress-ns
  annotations:
    werf.io/ownership: anyone
`

func TestReleaseInstall_CanceledDeployRunsFailurePlanAndAutoRollback(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)

	ctx, cancel := context.WithCancel(newStuckRevisionContext(t))
	defer cancel()

	api.onRequest = cancelOnRequest(cancel, http.MethodPatch)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newCanceledDeployChart(t, api)
	opts.AutoRollback = true

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
}

func TestReleaseInstall_TimedOutDeployRunsFailurePlanAndAutoRollback(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)

	var delayOnce sync.Once

	api.onRequest = func(r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Query().Get("dryRun") == "" && filepath.Base(r.URL.Path) == "canceled-resource" {
			delayOnce.Do(func() {
				time.Sleep(3 * time.Second)
			})
		}
	}

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newCanceledDeployChart(t, api)
	opts.AutoRollback = true
	opts.Timeout = 2 * time.Second

	_, err := ReleaseInstall(newStuckRevisionContext(t), "progress-release", "progress-ns", opts)
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
}

func TestReleaseRollback_CanceledRollbackRunsFailurePlan(t *testing.T) {
	api := newProgressPhasesAPI(t)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
	})
	addProgressPhaseRevisionWithStatus(t, api, 1, canceledResourceManifest, helmreleasestatus.StatusSuperseded)
	addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusDeployed)

	ctx, cancel := context.WithCancel(newStuckRevisionContext(t))
	defer cancel()

	api.onRequest = cancelOnRequest(cancel, http.MethodPatch)

	err := ReleaseRollback(ctx, "progress-release", "progress-ns", ReleaseRollbackOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:   true, NoShowNotes: true,
		ReleaseStorageDriver: "configmap",
		TempDirPath:          t.TempDir(),
	})
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusDeployed,
		3: helmreleasestatus.StatusFailed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
}

func TestReleaseUninstall_CanceledUninstallRunsFailurePlan(t *testing.T) {
	api := newProgressPhasesAPI(t)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.retainNamespace = true
	})
	addProgressPhaseRevisionWithStatus(t, api, 1, addReleaseConfigMap(t, api, "canceled-resource", nil), helmreleasestatus.StatusDeployed)

	ctx, cancel := context.WithCancel(newStuckRevisionContext(t))
	defer cancel()

	api.onRequest = cancelOnRequest(cancel, http.MethodDelete)

	opts := ReleaseUninstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:   true,
		TempDirPath:           t.TempDir(),
	}
	opts.ReleaseStorageDriver = "configmap"

	require.Error(t, ReleaseUninstall(ctx, "progress-release", "progress-ns", opts))

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusFailed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
}

func cancelOnRequest(cancel context.CancelFunc, method string) func(r *http.Request) {
	var cancelOnce sync.Once

	return func(r *http.Request) {
		if r.Method == method && r.URL.Query().Get("dryRun") == "" && filepath.Base(r.URL.Path) == "canceled-resource" {
			cancelOnce.Do(cancel)
		}
	}
}

func newCanceledDeployChart(t *testing.T, api *progressPhasesAPI) string {
	t.Helper()

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
	})

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(canceledResourceManifest), 0o600))

	return chartPath
}
