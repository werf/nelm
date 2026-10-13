package action

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmreleasev1 "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

func TestBuildReleaseGetResult(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{
		"owner":   "helm",
		"name":    "myrel",
		"status":  "deployed",
		"version": "3",
		"team":    "platform",
	}
	annotations := map[string]string{
		"nelm.werf.io/released-by": "nelm",
		"example.com/managed-by":   "platform",
	}
	relAccessor, err := helmrelease.NewAccessor(&helmreleasev1.Release{
		Name:      "myrel",
		Namespace: "ns",
		Version:   3,
		Labels:    labels,
		Info: &helmreleasev1.Info{
			Status:      helmreleasestatus.StatusDeployed,
			Notes:       "Release notes",
			Annotations: annotations,
		},
		Chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"},
			Values:   map[string]any{"default": "value", "override": "default"},
		},
		Config: map[string]any{"override": "configured"},
		Manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: myconfig
  namespace: ns
`,
	})
	require.NoError(t, err)

	result, err := buildReleaseGetResult(ctx, relAccessor, "ns")
	require.NoError(t, err)
	require.NotNil(t, result.Release)
	assert.Equal(t, "v3", result.APIVersion)
	assert.Equal(t, &ReleaseGetResultRelease{
		Name:        "myrel",
		Namespace:   "ns",
		Revision:    3,
		Status:      helmreleasestatus.StatusDeployed,
		DeployedAt:  &ReleaseGetResultDeployedAt{Human: time.Time{}.String(), Unix: int(time.Time{}.Unix())},
		Annotations: annotations,
		Labels:      labels,
	}, result.Release)
	assert.Equal(t, &ReleaseGetResultChart{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}, result.Chart)
	assert.Equal(t, "Release notes", result.Notes)
	assert.Equal(t, map[string]any{"default": "value", "override": "configured"}, result.Values)
	require.Len(t, result.ResourceSpecs, 1)

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var (
				data []byte
				err  error
			)
			switch format {
			case "json":
				data, err = json.Marshal(result)
			case "yaml":
				data, err = yaml.MarshalContext(ctx, result)
			}

			require.NoError(t, err)

			var output map[string]any
			switch format {
			case "json":
				require.NoError(t, json.Unmarshal(data, &output))
			case "yaml":
				require.NoError(t, yaml.UnmarshalContext(ctx, data, &output))
			}

			assert.Equal(t, "v3", output["apiVersion"])
			releaseOutput, ok := output["release"].(map[string]any)
			require.True(t, ok)
			assert.NotContains(t, releaseOutput, "storageLabels")
			assert.Equal(t, map[string]any{
				"owner": "helm", "name": "myrel", "status": "deployed", "version": "3", "team": "platform",
			}, releaseOutput["labels"])
			assert.Equal(t, map[string]any{
				"nelm.werf.io/released-by": "nelm", "example.com/managed-by": "platform",
			}, releaseOutput["annotations"])
			assert.Contains(t, output, "chart")
			assert.Equal(t, "Release notes", output["notes"])
			assert.Contains(t, output, "values")
			assert.Contains(t, output, "resourceSpecs")
		})
	}
}
