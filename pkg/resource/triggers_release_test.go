package resource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestNewInstallableResourceValidatesTriggersRelease(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "true", value: "true"},
		{name: "false", value: "false"},
		{name: "empty", value: "", wantErr: true},
		{name: "capitalized", value: "False", wantErr: true},
		{name: "numeric", value: "0", wantErr: true},
		{name: "garbage", value: "no", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":        "test-configmap",
					"annotations": map[string]interface{}{"werf.io/triggers-release": tt.value},
				},
			}}
			resSpec := spec.NewResourceSpec(obj, resourcePolicyTestNamespace, spec.ResourceSpecOptions{})

			_, err := resource.NewInstallableResource(context.Background(), resSpec, nil, resourcePolicyTestNamespace, resource.InstallableResourceOptions{})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), `invalid value "`+tt.value+`" for annotation "werf.io/triggers-release"`)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTriggersRelease(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "no annotation", annotations: nil, want: true},
		{name: "true", annotations: map[string]string{"werf.io/triggers-release": "true"}, want: true},
		{name: "false", annotations: map[string]string{"werf.io/triggers-release": "false"}, want: false},
		{name: "unvalidated non-false value", annotations: map[string]string{"werf.io/triggers-release": "False"}, want: true},
		{name: "padded false", annotations: map[string]string{"werf.io/triggers-release": " false "}, want: false},
		{name: "similar foreign key", annotations: map[string]string{"werf-io/triggers-release": "false"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resource.TriggersRelease(tt.annotations))
		})
	}
}
