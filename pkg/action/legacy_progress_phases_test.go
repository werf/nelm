package action

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestReleaseInstallProgressPhasesOnEarlyError(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "phases"}[enabled], func(t *testing.T) {
			reports := make(chan progrep.ProgressReport, 8)
			_, err := ReleaseInstall(context.Background(), "rel", "ns", ReleaseInstallOptions{
				AutoRollback:               true,
				LegacyProgressReportCh:     reports,
				LegacyProgressReportPhases: enabled,
				TempDirPath:                t.TempDir(),
			})
			require.Error(t, err)

			var snapshots []progrep.ProgressReport
			for report := range reports {
				snapshots = append(snapshots, report)
			}

			if !enabled {
				assert.Empty(t, snapshots)

				return
			}

			require.Len(t, snapshots, 2)

			for _, snapshot := range snapshots {
				assert.Equal(t, progrep.PhaseKindInitializing, snapshot.CurrentPhase)
				assert.Empty(t, snapshot.Operations)
				assert.Nil(t, snapshot.RenderDetails)
				assert.Nil(t, snapshot.AnalyzeDetails)
				assert.Nil(t, snapshot.NamespaceDeleteDetails)
			}
		})
	}
}

func TestReleaseUninstallProgressPhasesOnEarlyError(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "phases"}[enabled], func(t *testing.T) {
			reports := make(chan progrep.ProgressReport, 8)
			opts := ReleaseUninstallOptions{
				LegacyProgressReportCh:     reports,
				LegacyProgressReportPhases: enabled,
				TempDirPath:                t.TempDir(),
			}
			opts.ReleaseStorageDriver = common.ReleaseStorageDriverMemory

			err := ReleaseUninstall(context.Background(), "rel", "ns", opts)
			require.Error(t, err)

			var snapshots []progrep.ProgressReport
			for report := range reports {
				snapshots = append(snapshots, report)
			}

			if !enabled {
				assert.Empty(t, snapshots)

				return
			}

			require.Len(t, snapshots, 2)

			for _, snapshot := range snapshots {
				assert.Equal(t, progrep.PhaseKindInitializing, snapshot.CurrentPhase)
				assert.Empty(t, snapshot.Operations)
				assert.Nil(t, snapshot.RenderDetails)
				assert.Nil(t, snapshot.AnalyzeDetails)
				assert.Nil(t, snapshot.NamespaceDeleteDetails)
			}
		})
	}
}
