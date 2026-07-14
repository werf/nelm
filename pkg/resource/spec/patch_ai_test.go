//go:build ai_tests

package spec_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/dyntracker/statestore"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource/spec"
)

func TestAI_LegacyOnlyTrackJobsPatcher_Patch(t *testing.T) {
	defaultFailMode := string(statestore.IgnoreAndContinueDeployProcess)
	defaultTrackTermination := string(statestore.NonBlocking)

	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:  "injects both defaults when neither key is set",
			input: nil,
			expected: map[string]string{
				common.AnnotationKeyHumanFailMode:             defaultFailMode,
				common.AnnotationKeyHumanTrackTerminationMode: defaultTrackTermination,
			},
		},
		{
			name: "preserves both user-set values",
			input: map[string]string{
				common.AnnotationKeyHumanFailMode:             string(statestore.FailWholeDeployProcessImmediately),
				common.AnnotationKeyHumanTrackTerminationMode: string(statestore.WaitUntilResourceReady),
			},
			expected: map[string]string{
				common.AnnotationKeyHumanFailMode:             string(statestore.FailWholeDeployProcessImmediately),
				common.AnnotationKeyHumanTrackTerminationMode: string(statestore.WaitUntilResourceReady),
			},
		},
		{
			name: "preserves fail-mode override and injects track-termination default",
			input: map[string]string{
				common.AnnotationKeyHumanFailMode: string(statestore.FailWholeDeployProcessImmediately),
			},
			expected: map[string]string{
				common.AnnotationKeyHumanFailMode:             string(statestore.FailWholeDeployProcessImmediately),
				common.AnnotationKeyHumanTrackTerminationMode: defaultTrackTermination,
			},
		},
		{
			name: "preserves track-termination override and injects fail-mode default",
			input: map[string]string{
				common.AnnotationKeyHumanTrackTerminationMode: string(statestore.WaitUntilResourceReady),
			},
			expected: map[string]string{
				common.AnnotationKeyHumanFailMode:             defaultFailMode,
				common.AnnotationKeyHumanTrackTerminationMode: string(statestore.WaitUntilResourceReady),
			},
		},
		{
			name: "treats empty values as user overrides",
			input: map[string]string{
				common.AnnotationKeyHumanFailMode:             "",
				common.AnnotationKeyHumanTrackTerminationMode: "",
			},
			expected: map[string]string{
				common.AnnotationKeyHumanFailMode:             "",
				common.AnnotationKeyHumanTrackTerminationMode: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test",
				},
			}}
			if tt.input != nil {
				obj.SetAnnotations(tt.input)
			}

			patcher := spec.NewLegacyOnlyTrackJobsPatcher()

			out, err := patcher.Patch(context.Background(), &spec.ResourcePatcherResourceInfo{Obj: obj})
			require.NoError(t, err)

			annos := out.GetAnnotations()
			require.Equal(t, tt.expected[common.AnnotationKeyHumanFailMode], annos[common.AnnotationKeyHumanFailMode])
			require.Equal(t, tt.expected[common.AnnotationKeyHumanTrackTerminationMode], annos[common.AnnotationKeyHumanTrackTerminationMode])
		})
	}
}
