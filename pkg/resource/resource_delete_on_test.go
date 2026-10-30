package resource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/resource"
)

func TestNewDeletableResourceDeleteStages(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect map[common.On]common.Stage
	}{
		{
			name:  "pre stages",
			value: "pre-install,pre-upgrade,pre-rollback,pre-delete",
			expect: map[common.On]common.Stage{
				common.InstallOnInstall:  common.StagePreUninstall,
				common.InstallOnUpgrade:  common.StagePreUninstall,
				common.InstallOnRollback: common.StagePreUninstall,
				common.InstallOnDelete:   common.StagePreUninstall,
			},
		},
		{
			name:  "main stages",
			value: "install, upgrade, rollback, delete",
			expect: map[common.On]common.Stage{
				common.InstallOnInstall:  common.StageUninstall,
				common.InstallOnUpgrade:  common.StageUninstall,
				common.InstallOnRollback: common.StageUninstall,
				common.InstallOnDelete:   common.StageUninstall,
			},
		},
		{
			name:  "post stages",
			value: "post-install,post-upgrade,post-rollback,post-delete",
			expect: map[common.On]common.Stage{
				common.InstallOnInstall:  common.StagePostUninstall,
				common.InstallOnUpgrade:  common.StagePostUninstall,
				common.InstallOnRollback: common.StagePostUninstall,
				common.InstallOnDelete:   common.StagePostUninstall,
			},
		},
		{
			name:  "some deploy types",
			value: "post-upgrade",
			expect: map[common.On]common.Stage{
				common.InstallOnUpgrade: common.StagePostUninstall,
			},
		},
		{
			name:  "unsupported value falls back to default stages",
			value: "post-upgrade,test",
		},
		{
			name:  "multiple stages for one deploy type fall back to default stages",
			value: "pre-upgrade,post-upgrade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resSpec := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				"werf.io/delete-on": tt.value,
			})

			res, err := resource.NewDeletableResource(context.Background(), resSpec, nil, testReleaseNamespace, resource.DeletableResourceOptions{})
			require.NoError(t, err)
			require.Equal(t, tt.expect, res.DeleteStages)
		})
	}
}

func TestNewInstallableResourceValidatesDeleteOn(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError string
	}{
		{
			name:  "all deploy types",
			value: "pre-install,upgrade,post-rollback,delete",
		},
		{
			name:        "empty value",
			value:       "",
			expectError: "expected non-empty string value",
		},
		{
			name:        "empty element",
			value:       "upgrade,",
			expectError: "one of the comma-separated values is empty",
		},
		{
			name:        "unsupported value",
			value:       "pre-uninstall",
			expectError: `value "pre-uninstall" for annotation "werf.io/delete-on" is not supported`,
		},
		{
			name:        "multiple stages for one deploy type",
			value:       "pre-upgrade,install,post-upgrade",
			expectError: `"pre-upgrade" and "post-upgrade" can't be used together, only one stage per deploy type is allowed`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resSpec := newDependentConfigMapSpec(testReleaseNamespace, map[string]string{
				"werf.io/delete-on": tt.value,
			})

			_, err := resource.NewInstallableResource(context.Background(), resSpec, nil, testReleaseNamespace, resource.InstallableResourceOptions{})
			if tt.expectError == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, "validate delete stage")
			require.ErrorContains(t, err, tt.expectError)
		})
	}
}
