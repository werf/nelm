package resource_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/spec"
)

const resourcePolicyTestNamespace = "test-namespace"

func TestResourcePoliciesSkipDelete(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "no annotation", annotations: nil, want: false},
		{name: "helm.sh keep", annotations: map[string]string{"helm.sh/resource-policy": "keep"}, want: true},
		{name: "werf.io keep", annotations: map[string]string{"werf.io/resource-policy": "keep"}, want: true},
		{name: "werf.io skip-delete", annotations: map[string]string{"werf.io/resource-policy": "skip-delete"}, want: true},
		{name: "werf.io skip-create only", annotations: map[string]string{"werf.io/resource-policy": "skip-create"}, want: false},
		{name: "werf.io skip-delete among others", annotations: map[string]string{"werf.io/resource-policy": "skip-update,skip-delete"}, want: true},
		{name: "werf.io overrides helm.sh keep", annotations: map[string]string{"helm.sh/resource-policy": "keep", "werf.io/resource-policy": "skip-create"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lo.Contains(resource.ResourcePolicies(resourcePolicyMeta(tt.annotations), resourcePolicyTestNamespace), common.ResourcePolicySkipDelete)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateResourcePolicy(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantErr     bool
	}{
		{name: "no annotation", annotations: nil},
		{name: "helm.sh keep", annotations: map[string]string{"helm.sh/resource-policy": "keep"}},
		{name: "helm.sh empty is invalid", annotations: map[string]string{"helm.sh/resource-policy": ""}, wantErr: true},
		{name: "helm.sh skip-create is invalid", annotations: map[string]string{"helm.sh/resource-policy": "skip-create"}, wantErr: true},
		{name: "werf.io keep", annotations: map[string]string{"werf.io/resource-policy": "keep"}},
		{name: "werf.io skip-delete", annotations: map[string]string{"werf.io/resource-policy": "skip-delete"}},
		{name: "werf.io all directives", annotations: map[string]string{"werf.io/resource-policy": "skip-create,skip-update,skip-recreate,skip-delete,keep"}},
		{name: "werf.io with spaces", annotations: map[string]string{"werf.io/resource-policy": "skip-create, skip-update"}},
		{name: "werf.io empty is invalid", annotations: map[string]string{"werf.io/resource-policy": ""}, wantErr: true},
		{name: "werf.io empty segment is invalid", annotations: map[string]string{"werf.io/resource-policy": "skip-create,,skip-update"}, wantErr: true},
		{name: "werf.io unknown value is invalid", annotations: map[string]string{"werf.io/resource-policy": "skip-create,bogus"}, wantErr: true},
		// werf.io fully overrides helm.sh: a bad legacy value must not reject the resource.
		{name: "werf.io present ignores bad helm.sh value", annotations: map[string]string{"helm.sh/resource-policy": "bogus", "werf.io/resource-policy": "skip-update"}},
		{name: "werf.io invalid still rejected even with valid helm.sh", annotations: map[string]string{"helm.sh/resource-policy": "keep", "werf.io/resource-policy": "bogus"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resource.ValidateResourcePolicy(resourcePolicyMeta(tt.annotations))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func resourcePolicyMeta(annotations map[string]string) *spec.ResourceMeta {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test-configmap",
			},
		},
	}
	if len(annotations) > 0 {
		obj.SetAnnotations(annotations)
	}

	return spec.NewResourceMetaFromUnstructured(obj, resourcePolicyTestNamespace, "")
}
