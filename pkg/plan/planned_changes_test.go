package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
)

func TestCalculatePlannedChanges_BlindApplyReasonContainsDryApplyErr(t *testing.T) {
	localRes := lifecycleInstallable("cm", common.StageInit)
	info := &InstallableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
		DryApplyErr:   fmt.Errorf("server-side dry-run apply resource: %w", fmt.Errorf("forbidden")),
		MustInstall:   ResourceInstallTypeApply,
	}

	changes, err := CalculatePlannedChanges([]*InstallableResourceInfo{info}, nil)
	require.NoError(t, err)

	require.Len(t, changes, 1)
	assert.Equal(t, "blind apply", changes[0].Type)
	assert.Equal(t, "error: server-side dry-run apply resource: forbidden", changes[0].Reason)
}
