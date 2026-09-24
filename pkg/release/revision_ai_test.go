//go:build ai_tests

package release

import (
	"testing"

	"github.com/stretchr/testify/assert"

	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
)

func TestAI_DeployedRevisions(t *testing.T) {
	deployed := helmreleasecommon.StatusDeployed.String()
	superseded := helmreleasecommon.StatusSuperseded.String()
	failed := helmreleasecommon.StatusFailed.String()
	uninstalled := helmreleasecommon.StatusUninstalled.String()
	uninstalling := helmreleasecommon.StatusUninstalling.String()

	rev := func(version int, status string) Revision {
		return Revision{Name: "myrelease", Namespace: "test-ns", Version: version, Status: status}
	}

	tests := []struct {
		name     string
		input    []Revision
		expected []int
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "only failed",
			input:    []Revision{rev(1, failed), rev(2, failed)},
			expected: nil,
		},
		{
			name:     "deployed and superseded mixed with failed",
			input:    []Revision{rev(1, superseded), rev(2, failed), rev(3, deployed)},
			expected: []int{1, 3},
		},
		{
			name:     "uninstalled in the middle drops everything before it",
			input:    []Revision{rev(1, superseded), rev(2, uninstalled), rev(3, deployed)},
			expected: []int{3},
		},
		{
			name:     "uninstalled last yields nothing",
			input:    []Revision{rev(1, deployed), rev(2, uninstalled)},
			expected: nil,
		},
		{
			name:     "uninstalling last yields nothing",
			input:    []Revision{rev(1, deployed), rev(2, uninstalling)},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := DeployedRevisions(test.input)

			versions := make([]int, 0, len(result))
			for _, r := range result {
				versions = append(versions, r.Version)
			}

			if len(test.expected) == 0 {
				assert.Empty(t, versions)

				return
			}

			assert.Equal(t, test.expected, versions)
		})
	}
}
