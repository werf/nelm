package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource"
)

func TestGetSensitiveInfo(t *testing.T) {
	tests := []struct {
		name        string
		groupKind   schema.GroupKind
		annotations map[string]string
		expected    resource.SensitiveInfo
	}{
		{
			name:        "regular resource not sensitive",
			groupKind:   schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{},
			expected:    resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:        "secret resource automatically sensitive",
			groupKind:   schema.GroupKind{Group: "", Kind: "Secret"},
			annotations: map[string]string{},
			expected:    resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*"}},
		},
		{
			name:      "resource with sensitive annotation set to true",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive": "true",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{resource.HideAll}},
		},
		{
			name:      "resource with sensitive annotation set to false",
			groupKind: schema.GroupKind{Group: "", Kind: "Secret"},
			annotations: map[string]string{
				"werf.io/sensitive": "false",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*"}},
		},
		{
			name:      "resource with sensitive-paths annotation",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": `["spec.template.spec.containers.*.env.*.value", "data.password"]`,
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"spec.template.spec.containers.*.env.*.value", "data.password"}},
		},
		{
			name:      "resource with both sensitive and sensitive-paths annotations - sensitive takes precedence",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive":       "true",
				"werf.io/sensitive-paths": `["data.password"]`,
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{resource.HideAll}},
		},
		{
			name:      "resource with invalid sensitive-paths annotation",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": `invalid json`,
			},
			expected: resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:      "resource with empty sensitive-paths annotation",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": `[]`,
			},
			expected: resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resource.GetSensitiveInfo(tt.groupKind, tt.annotations)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSensitiveBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name        string
		groupKind   schema.GroupKind
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "regular resource not sensitive",
			groupKind:   schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name:        "secret resource automatically sensitive",
			groupKind:   schema.GroupKind{Group: "", Kind: "Secret"},
			annotations: map[string]string{},
			expected:    true,
		},
		{
			name:      "resource with sensitive annotation set to true",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive": "true",
			},
			expected: true,
		},
		{
			name:      "resource with sensitive-paths annotation",
			groupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": `["data.password"]`,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resource.IsSensitive(tt.groupKind, tt.annotations)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactSensitiveData(t *testing.T) {
	tests := []struct {
		name           string
		input          *unstructured.Unstructured
		sensitivePaths []string
		expectedFields map[string]interface{}
	}{
		{
			name: "no sensitive paths",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "test-secret",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"username": "dXNlcm5hbWU=",
						"password": "cGFzc3dvcmQ=",
					},
				},
			},
			sensitivePaths: []string{},
			expectedFields: map[string]interface{}{
				"data": map[string]interface{}{
					"username": "dXNlcm5hbWU=",
					"password": "cGFzc3dvcmQ=",
				},
			},
		},
		{
			name: "hide all",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "test-secret",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"username": "dXNlcm5hbWU=",
						"password": "cGFzc3dvcmQ=",
					},
				},
			},
			sensitivePaths: []string{resource.HideAll},
			expectedFields: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "test-secret",
					"namespace": "default",
				},
			},
		},
		{
			name: "redact data fields with wildcard",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "test-secret",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"username": "dXNlcm5hbWU=",
						"password": "cGFzc3dvcmQ=",
					},
					"type": "Opaque",
				},
			},
			sensitivePaths: []string{"data.*"},
			expectedFields: map[string]interface{}{
				"data": map[string]interface{}{
					"username": "REDACTED (len 12 bytes)",
					"password": "REDACTED (len 12 bytes)",
				},
				"type": "Opaque",
			},
		},
		{
			name: "redact specific field",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "test-secret",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"username": "dXNlcm5hbWU=",
						"password": "cGFzc3dvcmQ=",
					},
				},
			},
			sensitivePaths: []string{"data.password"},
			expectedFields: map[string]interface{}{
				"data": map[string]interface{}{
					"username": "dXNlcm5hbWU=",
					"password": "REDACTED (len 12 bytes)",
				},
			},
		},
		{
			name: "redact nested fields",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "app",
										"env": []interface{}{
											map[string]interface{}{
												"name":  "DB_PASSWORD",
												"value": "secret123",
											},
											map[string]interface{}{
												"name":  "API_KEY",
												"value": "key456",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			sensitivePaths: []string{"spec.template.spec.containers.0.env.0.value"},
			expectedFields: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name": "app",
									"env": []interface{}{
										map[string]interface{}{
											"name":  "DB_PASSWORD",
											"value": "REDACTED (len 9 bytes)",
										},
										map[string]interface{}{
											"name":  "API_KEY",
											"value": "key456",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resource.RedactSensitiveData(tt.input, tt.sensitivePaths)

			if tt.name == "hide all" {
				// For hide all, check that only basic metadata is present
				assert.Equal(t, tt.expectedFields["apiVersion"], result.Object["apiVersion"])
				assert.Equal(t, tt.expectedFields["kind"], result.Object["kind"])
				assert.Equal(t, tt.expectedFields["metadata"], result.Object["metadata"])
				assert.NotContains(t, result.Object, "data")
				assert.NotContains(t, result.Object, "spec")
			} else {
				// For specific field redaction, check expected fields
				for key, expectedValue := range tt.expectedFields {
					assert.Equal(t, expectedValue, result.Object[key], "Field %s should match expected value", key)
				}
			}

			// Ensure original object is not modified
			if tt.name != "no sensitive paths" {
				assert.NotEqual(t, tt.input.Object, result.Object, "Original object should not be modified")
			}
		})
	}
}
