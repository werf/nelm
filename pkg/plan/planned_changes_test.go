package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/resource"
)

func TestCalculatePlannedChanges_BlindApplyReasonContainsDryApplyErr(t *testing.T) {
	resSpec := makeResourceSpec("cm", "default", gvkConfigMap)
	resSpec.Unstruct = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "cm", "namespace": "default"},
	}}
	info := &InstallableResourceInfo{
		ResourceMeta:  resSpec.ResourceMeta,
		LocalResource: &resource.InstallableResource{ResourceSpec: resSpec},
		DryApplyErr:   fmt.Errorf("server-side dry-run apply resource: %w", fmt.Errorf("forbidden")),
		MustInstall:   ResourceInstallTypeApply,
	}

	changes, err := CalculatePlannedChanges([]*InstallableResourceInfo{info}, nil)
	require.NoError(t, err)

	require.Len(t, changes, 1)
	assert.Equal(t, "blind apply", changes[0].Type)
	assert.Equal(t, "error: server-side dry-run apply resource: forbidden", changes[0].Reason)
}
