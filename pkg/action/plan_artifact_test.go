package action

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/v2/pkg/plan"
)

func TestPlanArtifact_RoundTripsInstallableResourceInfoWithDryApplyErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.gz")
	dryApplyErr := fmt.Errorf("server-side dry-run apply resource: %w", apierrors.NewForbidden(
		schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: "clusterroles"},
		"example", fmt.Errorf("cannot patch resource")))

	artifact := &PlanArtifact{
		APIVersion: PlanArtifactSchemeVersion,
		Data: &PlanArtifactData{
			Changes: []*plan.ResourceChange{{Reason: fmt.Sprintf("error: %s", dryApplyErr)}},
			InstallableResourceInfos: []*plan.InstallableResourceInfo{
				{DryApplyErr: dryApplyErr, MustInstall: plan.ResourceInstallTypeApply},
				{DryApplyErr: apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "example", fmt.Errorf("denied")), MustInstall: plan.ResourceInstallTypeApply},
			},
		},
	}

	require.NoError(t, WritePlanArtifact(context.Background(), artifact, path, "", ""))

	readArtifact, err := ReadPlanArtifact(context.Background(), path, "", "")
	require.NoError(t, err)

	require.Len(t, readArtifact.Data.InstallableResourceInfos, 2)

	for _, info := range readArtifact.Data.InstallableResourceInfos {
		require.NoError(t, info.DryApplyErr)
		assert.Equal(t, plan.ResourceInstallTypeApply, info.MustInstall)
	}

	require.Len(t, readArtifact.Data.Changes, 1)
	assert.Equal(t, fmt.Sprintf("error: %s", dryApplyErr), readArtifact.Data.Changes[0].Reason)
}

func TestReadPlanArtifact_ReadsArtifactWithLegacySerializedDryApplyErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.gz")
	dataRaw := `{"installableResourceInfos":[{"dryApplyErr":{},"mustInstall":"apply"},{"dryApplyErr":{"ErrStatus":{"status":"Failure","reason":"Forbidden","code":403}},"mustInstall":"apply"}]}`

	file, err := os.Create(path)
	require.NoError(t, err)

	gzipWriter := gzip.NewWriter(file)
	require.NoError(t, json.NewEncoder(gzipWriter).Encode(&PlanArtifact{APIVersion: PlanArtifactSchemeVersion, DataRaw: dataRaw}))
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, file.Close())

	readArtifact, err := ReadPlanArtifact(context.Background(), path, "", "")
	require.NoError(t, err)

	require.Len(t, readArtifact.Data.InstallableResourceInfos, 2)

	for _, info := range readArtifact.Data.InstallableResourceInfos {
		require.NoError(t, info.DryApplyErr)
		assert.Equal(t, plan.ResourceInstallTypeApply, info.MustInstall)
	}
}
