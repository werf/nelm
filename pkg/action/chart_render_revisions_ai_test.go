//go:build ai_tests

package action

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/pkg/common"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/pkg/release"
)

func TestAI_ResolveDeployState(t *testing.T) {
	for _, tt := range []struct {
		name           string
		revisions      []release.Revision
		wantRevision   int
		wantDeployType common.DeployType
	}{
		{
			name:           "no revisions",
			revisions:      nil,
			wantRevision:   1,
			wantDeployType: common.DeployTypeInitial,
		},
		{
			name: "only failed revisions",
			revisions: []release.Revision{
				{Version: 1, Status: helmreleasecommon.StatusFailed.String()},
				{Version: 2, Status: helmreleasecommon.StatusFailed.String()},
			},
			wantRevision:   3,
			wantDeployType: common.DeployTypeInstall,
		},
		{
			name: "last revision deployed",
			revisions: []release.Revision{
				{Version: 1, Status: helmreleasecommon.StatusSuperseded.String()},
				{Version: 2, Status: helmreleasecommon.StatusDeployed.String()},
			},
			wantRevision:   3,
			wantDeployType: common.DeployTypeUpgrade,
		},
		{
			name: "deployed followed by failed",
			revisions: []release.Revision{
				{Version: 1, Status: helmreleasecommon.StatusDeployed.String()},
				{Version: 2, Status: helmreleasecommon.StatusFailed.String()},
			},
			wantRevision:   3,
			wantDeployType: common.DeployTypeUpgrade,
		},
		{
			name: "superseded without deployed",
			revisions: []release.Revision{
				{Version: 1, Status: helmreleasecommon.StatusSuperseded.String()},
				{Version: 2, Status: helmreleasecommon.StatusFailed.String()},
			},
			wantRevision:   3,
			wantDeployType: common.DeployTypeUpgrade,
		},
		{
			name: "uninstalled last",
			revisions: []release.Revision{
				{Version: 1, Status: helmreleasecommon.StatusSuperseded.String()},
				{Version: 2, Status: helmreleasecommon.StatusUninstalled.String()},
			},
			wantRevision:   3,
			wantDeployType: common.DeployTypeInstall,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newRevision, deployType := resolveDeployState(tt.revisions)

			assert.Equal(t, tt.wantRevision, newRevision)
			assert.Equal(t, tt.wantDeployType, deployType)
		})
	}
}
