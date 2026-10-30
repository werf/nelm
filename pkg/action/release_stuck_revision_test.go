package action

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

func TestReleaseInstall_AutoRollbackMarksStuckPendingRevisionsFailed(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusPendingUpgrade)

	ctx := newStuckRevisionContext(t)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newStuckRevisionFailingChart(t, api)
	opts.AutoRollback = true

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusFailed,
		4: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, ctx, api))
}

func TestReleaseInstall_FailureKeepsStuckPendingRevisions(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusPendingUpgrade)

	ctx := newStuckRevisionContext(t)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newStuckRevisionFailingChart(t, api)

	_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.Error(t, err)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusDeployed,
		2: helmreleasestatus.StatusPendingUpgrade,
		3: helmreleasestatus.StatusFailed,
	}, progressPhaseRevisionStatuses(t, ctx, api))
}

func TestReleaseInstall_MarkingStuckRevisionFailedChangesOnlyItsStatus(t *testing.T) {
	for _, usePlanArtifactFile := range []bool{false, true} {
		t.Run("plan artifact file "+strconv.FormatBool(usePlanArtifactFile), func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
			stuckBody := addStuckRevisionWithFullBody(t, api, 2)
			chartPath := newStuckRevisionChart(t, api)

			ctx := newStuckRevisionContext(t)

			opts := newStuckRevisionInstallOptions(t, api)
			if usePlanArtifactFile {
				opts.PlanArtifactPath = filepath.Join(t.TempDir(), "plan.gz")
				opts.PlanArtifactLifetime = time.Hour

				_, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
					KubeConnectionOptions: api.kubeOpts, Chart: chartPath, PlanArtifactPath: opts.PlanArtifactPath,
					ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"},
					TempDirPath:                  t.TempDir(),
				})
				require.NoError(t, err)
			} else {
				opts.Chart = chartPath
			}

			_, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
			require.NoError(t, err)

			var storedBody []byte
			api.state.RTransaction(func(s *progressPhasesAPIState) {
				storedBody = decodeStoredReleaseBody(t, s.configMaps["sh.helm.release.v1.progress-release.v2"].Data["release"])
			})

			var want, got map[string]any
			require.NoError(t, json.Unmarshal(stuckBody, &want))
			require.NoError(t, json.Unmarshal(storedBody, &got))

			assert.Equal(t, helmreleasestatus.StatusFailed.String(), got["info"].(map[string]any)["status"])

			delete(want["info"].(map[string]any), "status")
			delete(got["info"].(map[string]any), "status")
			assert.Equal(t, want, got, "only the status of the stuck revision may change")
		})
	}
}

func TestReleaseInstall_StuckPendingRevisionPrunedByHistoryLimitDoesNotFailDeploy(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusPendingUpgrade)

	ctx := newStuckRevisionContext(t)

	opts := newStuckRevisionInstallOptions(t, api)
	opts.Chart = newStuckRevisionChart(t, api)
	opts.ReleaseHistoryLimit = 2

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.updateMissingConfigMapNotFound = true
	})

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
	require.NoError(t, err)
	assert.Equal(t, helmreleasestatus.StatusDeployed, result.Release.Status)

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, ctx, api))
}

func TestReleaseInstall_SuccessMarksStuckPendingRevisionsFailed(t *testing.T) {
	for _, usePlanArtifact := range []bool{false, true} {
		t.Run("plan artifact "+strconv.FormatBool(usePlanArtifact), func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
			addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusPendingUpgrade)
			addProgressPhaseRevisionWithStatus(t, api, 3, "", helmreleasestatus.StatusFailed)
			addProgressPhaseRevisionWithStatus(t, api, 4, "", helmreleasestatus.StatusPendingRollback)
			chartPath := newStuckRevisionChart(t, api)

			ctx := newStuckRevisionContext(t)

			opts := newStuckRevisionInstallOptions(t, api)
			if usePlanArtifact {
				planArtifact, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
					KubeConnectionOptions: api.kubeOpts, Chart: chartPath,
					ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"},
					TempDirPath:                  t.TempDir(),
				})
				require.NoError(t, err)

				opts.LegacyPlanArtifact = planArtifact
				opts.PlanArtifactLifetime = time.Hour
			} else {
				opts.Chart = chartPath
			}

			result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", opts)
			require.NoError(t, err)
			assert.Equal(t, helmreleasestatus.StatusDeployed, result.Release.Status)

			assert.Equal(t, map[int]helmreleasestatus.Status{
				1: helmreleasestatus.StatusSuperseded,
				2: helmreleasestatus.StatusFailed,
				3: helmreleasestatus.StatusFailed,
				4: helmreleasestatus.StatusFailed,
				5: helmreleasestatus.StatusDeployed,
			}, progressPhaseRevisionStatuses(t, ctx, api))
		})
	}
}

func TestReleaseRollback_SuccessMarksStuckPendingRevisionsFailed(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevisionWithStatus(t, api, 1, "", helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, "", helmreleasestatus.StatusPendingInstall)

	ctx := newStuckRevisionContext(t)

	require.NoError(t, ReleaseRollback(ctx, "progress-release", "progress-ns", ReleaseRollbackOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:   true, NoShowNotes: true,
		ReleaseStorageDriver: "configmap",
		TempDirPath:          t.TempDir(),
	}))

	assert.Equal(t, map[int]helmreleasestatus.Status{
		1: helmreleasestatus.StatusSuperseded,
		2: helmreleasestatus.StatusFailed,
		3: helmreleasestatus.StatusDeployed,
	}, progressPhaseRevisionStatuses(t, ctx, api))
}

func addStuckRevisionWithFullBody(t *testing.T, api *progressPhasesAPI, revision int) []byte {
	t.Helper()

	deployedAt := time.Date(2026, time.October, 1, 10, 0, 0, 0, time.UTC)
	body, err := json.Marshal(&helmrelease.Release{
		Name:      "progress-release",
		Namespace: "progress-ns",
		Version:   revision,
		Info: &helmrelease.Info{
			Annotations:   map[string]string{"werf.io/release-version": "v1", "example.com/team": "platform"},
			FirstDeployed: deployedAt,
			LastDeployed:  deployedAt.Add(time.Hour),
			Description:   "Upgrade in progress",
			Status:        helmreleasestatus.StatusPendingUpgrade,
			Notes:         "notes",
		},
		Chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{APIVersion: "v2", Name: "progress-chart", Version: "1.0.0"},
			Values:   map[string]any{"default": "value"},
		},
		Config:   map[string]any{"key": "value", "nested": map[string]any{"number": 42, "list": []any{"a", true}}},
		Manifest: "",
	})
	require.NoError(t, err)

	key := fmt.Sprintf("sh.helm.release.v1.progress-release.v%d", revision)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.configMaps[key] = corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: key, Namespace: "progress-ns", Annotations: map[string]string{
				"werf.io/release-version": "v1", "example.com/team": "platform",
			}, Labels: map[string]string{
				"owner": "helm", "name": "progress-release", "version": strconv.Itoa(revision), "status": helmreleasestatus.StatusPendingUpgrade.String(),
			}},
			Data: map[string]string{"release": base64.StdEncoding.EncodeToString(body)},
		}
	})

	return body
}

func decodeStoredReleaseBody(t *testing.T, data string) []byte {
	t.Helper()

	body, err := base64.StdEncoding.DecodeString(data)
	require.NoError(t, err)

	if !bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		return body
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)

	body, err = io.ReadAll(gzipReader)
	require.NoError(t, err)

	return body
}

func newStuckRevisionChart(t *testing.T, api *progressPhasesAPI) string {
	t.Helper()

	manifest := addProgressPhasesConfigMap(t, api)

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(manifest), 0o600))

	return chartPath
}

func newStuckRevisionContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 20*time.Second)
	t.Cleanup(cancel)

	return ctx
}

// The failing resource has anyone-ownership: a failed apply caches its error in the client, and a
// rollback that had to delete the resource would read that error back and fail as well.
func newStuckRevisionFailingChart(t *testing.T, api *progressPhasesAPI) string {
	t.Helper()

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.applyConfigMapStatus = map[string]int{"failing-resource": http.StatusForbidden}
	})

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: failing-resource
  annotations:
    werf.io/ownership: anyone
`), 0o600))

	return chartPath
}

func newStuckRevisionInstallOptions(t *testing.T, api *progressPhasesAPI) ReleaseInstallOptions {
	t.Helper()

	return ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		NoCreateNamespace:     true, LegacyNoReleaseLock: true, NoShowNotes: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"},
		TempDirPath:                  t.TempDir(),
	}
}

func progressPhaseRevisionStatuses(t *testing.T, ctx context.Context, api *progressPhasesAPI) map[int]helmreleasestatus.Status {
	t.Helper()

	labelStatuses := map[int]helmreleasestatus.Status{}
	api.state.RTransaction(func(s *progressPhasesAPIState) {
		for name, cm := range s.configMaps {
			if !strings.HasPrefix(name, "sh.helm.release.v1.progress-release.v") {
				continue
			}

			version, err := strconv.Atoi(cm.Labels["version"])
			require.NoError(t, err)

			labelStatuses[version] = helmreleasestatus.Status(cm.Labels["status"])
		}
	})

	for version, labelStatus := range labelStatuses {
		result, err := ReleaseGet(ctx, "progress-release", "progress-ns", ReleaseGetOptions{
			KubeConnectionOptions: api.kubeOpts, OutputNoPrint: true, ReleaseStorageDriver: "configmap",
			Revision: version, TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		assert.Equal(t, labelStatus, result.Release.Status, "stored body and storage label of revision %d disagree", version)
	}

	return labelStatuses
}
