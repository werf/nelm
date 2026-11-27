package action

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseInstall_CanceledDeployRunsFailurePlanAndAutoRollback(t *testing.T) {
	api := newProgressPhasesAPI(t)
	keptManifest := addReleaseConfigMap(t, api, "kept-resource", nil)
	addProgressPhaseRevision(t, api, 1, keptManifest)

	ctx, cancel := context.WithCancel(newStuckRevisionContext(t))
	defer cancel()

	api.onRequest = cancelOnRequest(cancel, http.MethodPatch)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newCanceledDeployChart(t, api, keptManifest)
	opts.AutoRollback = true

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
	assert.Equal(t, []string{"changed", "value"}, appliedKeptResourceValues(api), "the deploy must change the resource and the rollback must restore it")
}

func TestReleaseInstall_TimedOutDeployRunsFailurePlanAndAutoRollback(t *testing.T) {
	api := newProgressPhasesAPI(t)
	keptManifest := addReleaseConfigMap(t, api, "kept-resource", nil)
	addProgressPhaseRevision(t, api, 1, keptManifest)

	unblockCh := make(chan struct{})
	t.Cleanup(func() {
		close(unblockCh)
	})

	var blockOnce sync.Once

	api.onRequest = func(r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Query().Get("dryRun") == "" && filepath.Base(r.URL.Path) == "canceled-resource" {
			blockOnce.Do(func() {
				<-unblockCh
			})
		}
	}

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newCanceledDeployChart(t, api, keptManifest)
	opts.AutoRollback = true
	opts.Timeout = 5 * time.Second

	_, err := ReleaseInstall(newStuckRevisionContext(t), "progress-release", "progress-ns", opts)
	require.ErrorContains(t, err, "action timed out after 5s")

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, newStuckRevisionContext(t), api))
	assert.Equal(t, []string{"changed", "value"}, appliedKeptResourceValues(api), "the deploy must change the resource and the rollback must restore it")
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
	addProgressPhaseRevision(t, api, 1, addReleaseConfigMap(t, api, "canceled-resource", nil))

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
