package resource_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/featgate"
)

func TestGetSensitiveInfo(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv(featgate.FeatGateFieldSensitive.EnvVarName())
	defer func() {
		if originalEnv != "" {
			t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), originalEnv)
		}
	}()

	tests := []struct {
		name          string
		enableFeature bool
		groupKind     schema.GroupKind
		annotations   map[string]string
		expected      resource.SensitiveInfo
	}{
		{
			name:          "regular resource not sensitive",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations:   map[string]string{},
			expected:      resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:          "secret resource automatically sensitive - legacy behavior",
			enableFeature: false,
			groupKind:     schema.GroupKind{Group: "", Kind: "Secret"},
			annotations:   map[string]string{},
			expected:      resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"$$HIDE_ALL$$"}},
		},
		{
			name:          "secret resource with annotation - legacy behavior",
			enableFeature: false,
			groupKind:     schema.GroupKind{Group: "", Kind: "Secret"},
			annotations: map[string]string{
				"werf.io/sensitive": "false",
			},
			expected: resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:          "secret with sensitive annotation set to false",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "", Kind: "Secret"},
			annotations: map[string]string{
				"werf.io/sensitive": "false",
			},
			expected: resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:          "secret resource automatically sensitive - new behavior",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "", Kind: "Secret"},
			annotations:   map[string]string{},
			expected:      resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*", "stringData.*"}},
		},
		{
			name:          "resource with sensitive annotation set to true - legacy behavior",
			enableFeature: false,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive": "true",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{resource.HideAll}},
		},
		{
			name:          "resource with sensitive annotation set to true - new behavior",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive": "true",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*", "stringData.*"}},
		},
		{
			name:          "resource with comma-separated sensitive-paths annotation",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": "spec.template.spec.containers.*.env.*.value,data.password",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"spec.template.spec.containers.*.env.*.value", "data.password"}},
		},
		{
			name:          "resource with escaped comma in sensitive-paths",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": "data.field\\,with\\,commas,spec.other",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.field,with,commas", "spec.other"}},
		},
		{
			name:          "resource with both sensitive and sensitive-paths annotations - sensitive path precedence in v2",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive":       "true",
				"werf.io/sensitive-paths": "data.password",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.password"}},
		},
		{
			name:          "resource with empty sensitive-paths annotation",
			enableFeature: true,
			groupKind:     schema.GroupKind{Group: "apps", Kind: "Deployment"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": "",
			},
			expected: resource.SensitiveInfo{IsSensitive: false, SensitivePaths: nil},
		},
		{
			name:          "resource with sensitive-paths annotation - feature flag disabled",
			enableFeature: false,
			groupKind:     schema.GroupKind{Group: "v1", Kind: "ConfigMap"},
			annotations: map[string]string{
				"werf.io/sensitive-paths": "$.data[*]",
			},
			expected: resource.SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"$.data[*]"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set feature gate
			if tt.enableFeature {
				t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), "true")
			}

			result := resource.GetSensitiveInfo(tt.groupKind, tt.annotations)

			assert.Equal(t, tt.expected, result, "behavior should match expected")
		})
	}
}

func TestParseSensitivePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single path",
			input:    "data.password",
			expected: []string{"data.password"},
		},
		{
			name:     "multiple paths",
			input:    "data.password,spec.template.spec.containers.*.env.*.value",
			expected: []string{"data.password", "spec.template.spec.containers.*.env.*.value"},
		},
		{
			name:     "paths with spaces",
			input:    " data.password , spec.template ",
			expected: []string{"data.password", "spec.template"},
		},
		{
			name:     "escaped commas",
			input:    "data.field\\,with\\,commas,spec.other",
			expected: []string{"data.field,with,commas", "spec.other"},
		},
		{
			name:     "multiple escaped commas",
			input:    "data.a\\,b\\,c,spec.d\\,e,metadata.f",
			expected: []string{"data.a,b,c", "spec.d,e", "metadata.f"},
		},
		{
			name:     "trailing comma",
			input:    "data.password,spec.template,",
			expected: []string{"data.password", "spec.template"},
		},
		{
			name:     "empty segments",
			input:    "data.password,,spec.template",
			expected: []string{"data.password", "spec.template"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resource.ParseSensitivePaths(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactSensitiveData(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv(featgate.FeatGateFieldSensitive.EnvVarName())
	defer func() {
		if originalEnv != "" {
			t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), originalEnv)
		}
	}()

	tests := []struct {
		name           string
		enableFeature  bool
		input          *unstructured.Unstructured
		sensitivePaths []string
		checkFunc      func(t *testing.T, result *unstructured.Unstructured)
	}{
		{
			name:          "bug: sensitive-paths ignored when feature flag disabled",
			enableFeature: false,
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key1": "sensitive-value-1",
						"key2": "sensitive-value-2",
					},
				},
			},
			sensitivePaths: []string{"data.*"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				// The bug is that when feature flag is disabled, the entire data section
				// is removed instead of redacting only the specified sensitive paths
				data, found, err := unstructured.NestedMap(result.Object, "data")
				require.NoError(t, err)

				if !found {
					t.Errorf("data section was completely removed instead of being redacted")
				} else {
					// If data exists, it should be redacted
					for key, value := range data {
						valueStr, ok := value.(string)
						if ok && !strings.Contains(valueStr, "sensitive") {
							t.Errorf("Expected data.%s to be redacted but got: %s", key, valueStr)
						}
					}
				}
			},
		},
		{
			name:          "no sensitive paths",
			enableFeature: true,
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
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})
				assert.Equal(t, "dXNlcm5hbWU=", data["username"])
				assert.Equal(t, "cGFzc3dvcmQ=", data["password"])
			},
		},
		{
			name:          "hide all with feature gate",
			enableFeature: true,
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
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				assert.Equal(t, "v1", result.Object["apiVersion"])
				assert.Equal(t, "Secret", result.Object["kind"])
				metadata := result.Object["metadata"].(map[string]interface{})
				assert.Equal(t, "test-secret", metadata["name"])
				assert.Equal(t, "default", metadata["namespace"])
				assert.NotContains(t, result.Object, "data")
			},
		},
		{
			name:          "redact data fields with wildcard - new behavior",
			enableFeature: true,
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
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})
				usernameVal := data["username"].(string)
				passwordVal := data["password"].(string)

				// Check that values are replaced with sensitive format
				assert.Contains(t, usernameVal, "sensitive")
				assert.Contains(t, usernameVal, "12 sensitive bytes")
				assert.Contains(t, passwordVal, "sensitive")
				assert.Contains(t, passwordVal, "12 sensitive bytes")

				// Check that type field is preserved
				assert.Equal(t, "Opaque", result.Object["type"])
			},
		},
		{
			name:          "redact specific field",
			enableFeature: true,
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
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})

				// Username should be unchanged
				assert.Equal(t, "dXNlcm5hbWU=", data["username"])

				// Password should be redacted
				passwordVal := data["password"].(string)
				assert.Contains(t, passwordVal, "sensitive")
				assert.Contains(t, passwordVal, "12 sensitive bytes")
			},
		},
		{
			name:          "type change handling - string to slice",
			enableFeature: true,
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
												"name":  "CONFIG",
												"value": []interface{}{"item1", "item2", "item3"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			sensitivePaths: []string{"spec.template.spec.containers[0].env[0].value"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				spec := result.Object["spec"].(map[string]interface{})
				template := spec["template"].(map[string]interface{})
				templateSpec := template["spec"].(map[string]interface{})
				containers := templateSpec["containers"].([]interface{})
				container := containers[0].(map[string]interface{})
				env := container["env"].([]interface{})
				envVar := env[0].(map[string]interface{})

				valueStr := envVar["value"].(string)
				assert.Contains(t, valueStr, "sensitive")
				assert.Contains(t, valueStr, "sensitive entries")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set feature gate
			if tt.enableFeature {
				t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), "true")
			}

			result := resource.RedactSensitiveData(tt.input, tt.sensitivePaths)

			// Ensure original object is not modified
			assert.NotSame(t, tt.input, result, "Original object should not be modified")

			tt.checkFunc(t, result)
		})
	}
}

func TestSHA256HashingConsistency(t *testing.T) {
	// Enable feature gate
	originalEnv := os.Getenv(featgate.FeatGateFieldSensitive.EnvVarName())
	defer func() {
		if originalEnv != "" {
			t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), originalEnv)
		}
	}()

	t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), "true")

	input1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"data": map[string]interface{}{
				"password": "secret123",
			},
		},
	}

	input2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"data": map[string]interface{}{
				"password": "secret123",
			},
		},
	}

	result1 := resource.RedactSensitiveData(input1, []string{"data.password"})
	result2 := resource.RedactSensitiveData(input2, []string{"data.password"})

	data1 := result1.Object["data"].(map[string]interface{})
	data2 := result2.Object["data"].(map[string]interface{})

	// Same input should produce same hash
	assert.Equal(t, data1["password"], data2["password"], "Same input should produce same redacted output")

	// Different inputs should produce different hashes
	input3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"data": map[string]interface{}{
				"password": "different123",
			},
		},
	}

	result3 := resource.RedactSensitiveData(input3, []string{"data.password"})
	data3 := result3.Object["data"].(map[string]interface{})

	assert.NotEqual(t, data1["password"], data3["password"], "Different inputs should produce different redacted outputs")
}

func TestRedactAtJSONPath(t *testing.T) {
	// Enable feature gate
	originalEnv := os.Getenv(featgate.FeatGateFieldSensitive.EnvVarName())
	defer func() {
		if originalEnv != "" {
			t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), originalEnv)
		}
	}()

	t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), "true")

	tests := []struct {
		name           string
		input          *unstructured.Unstructured
		sensitivePaths []string
		checkFunc      func(t *testing.T, result *unstructured.Unstructured)
	}{
		{
			name: "nested object paths",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"data": map[string]interface{}{
						"config.yaml": "password: secret123\nuser: admin",
						"app.conf":    "db_password=mysecret",
					},
					"metadata": map[string]interface{}{
						"name": "test-config",
					},
				},
			},
			sensitivePaths: []string{"data['config.yaml']"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})
				configYaml := data["config.yaml"].(string)
				appConf := data["app.conf"].(string)

				assert.Contains(t, configYaml, "sensitive")
				assert.Contains(t, configYaml, "sensitive bytes")
				assert.Equal(t, "db_password=mysecret", appConf) // unchanged
			},
		},
		{
			name: "array elements with wildcard",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app1",
								"image": "nginx:latest",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "PASSWORD",
										"value": "secret123",
									},
									map[string]interface{}{
										"name":  "USER",
										"value": "admin",
									},
								},
							},
							map[string]interface{}{
								"name":  "app2",
								"image": "alpine:latest",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DB_PASSWORD",
										"value": "dbsecret456",
									},
								},
							},
						},
					},
				},
			},
			sensitivePaths: []string{"spec.containers.*.env.*.value"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				spec := result.Object["spec"].(map[string]interface{})
				containers := spec["containers"].([]interface{})

				// Check first container
				container1 := containers[0].(map[string]interface{})
				env1 := container1["env"].([]interface{})
				env1_0 := env1[0].(map[string]interface{})
				env1_1 := env1[1].(map[string]interface{})

				assert.Contains(t, env1_0["value"].(string), "sensitive")
				assert.Contains(t, env1_1["value"].(string), "sensitive")

				// Check second container
				container2 := containers[1].(map[string]interface{})
				env2 := container2["env"].([]interface{})
				env2_0 := env2[0].(map[string]interface{})

				assert.Contains(t, env2_0["value"].(string), "sensitive")

				// Verify names are unchanged
				assert.Equal(t, "PASSWORD", env1_0["name"])
				assert.Equal(t, "USER", env1_1["name"])
				assert.Equal(t, "DB_PASSWORD", env2_0["name"])
			},
		},
		{
			name: "specific array index",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"data": map[string]interface{}{
						"items": []interface{}{
							"public_item",
							"secret_item",
							"another_public_item",
						},
					},
				},
			},
			sensitivePaths: []string{"data.items[1]"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})
				items := data["items"].([]interface{})

				assert.Equal(t, "public_item", items[0])
				assert.Contains(t, items[1].(string), "sensitive")
				assert.Equal(t, "another_public_item", items[2])
			},
		},
		{
			name: "complex nested structure",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"volumes": []interface{}{
									map[string]interface{}{
										"name": "config-volume",
										"configMap": map[string]interface{}{
											"name": "my-config",
										},
									},
									map[string]interface{}{
										"name": "secret-volume",
										"secret": map[string]interface{}{
											"secretName": "my-secret",
											"items": []interface{}{
												map[string]interface{}{
													"key":  "password",
													"path": "db/password",
												},
												map[string]interface{}{
													"key":  "username",
													"path": "db/username",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			sensitivePaths: []string{"spec.template.spec.volumes[1].secret.items.*.key"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				spec := result.Object["spec"].(map[string]interface{})
				template := spec["template"].(map[string]interface{})
				templateSpec := template["spec"].(map[string]interface{})
				volumes := templateSpec["volumes"].([]interface{})

				// First volume should be unchanged
				volume0 := volumes[0].(map[string]interface{})
				assert.Equal(t, "config-volume", volume0["name"])

				// Second volume's secret items keys should be redacted
				volume1 := volumes[1].(map[string]interface{})
				secret := volume1["secret"].(map[string]interface{})
				items := secret["items"].([]interface{})

				item0 := items[0].(map[string]interface{})
				item1 := items[1].(map[string]interface{})

				assert.Contains(t, item0["key"].(string), "sensitive")
				assert.Contains(t, item1["key"].(string), "sensitive")

				// Paths should remain unchanged
				assert.Equal(t, "db/password", item0["path"])
				assert.Equal(t, "db/username", item1["path"])
			},
		},
		{
			name: "mixed data types",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"data": map[string]interface{}{
						"stringValue": "sensitive_string",
						"intValue":    "42",
						"boolValue":   "true",
						"arrayValue":  []interface{}{"item1", "item2", "item3"},
						"objectValue": map[string]interface{}{
							"nested": "nested_value",
						},
					},
				},
			},
			sensitivePaths: []string{"data.*"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})

				// All values should be redacted with appropriate sensitive format
				for key, value := range data {
					valueStr := value.(string)
					assert.Contains(t, valueStr, "sensitive", "Key %s should be redacted", key)

					switch key {
					case "stringValue":
						assert.Contains(t, valueStr, "16 sensitive bytes")
					case "intValue":
						assert.Contains(t, valueStr, "2 sensitive bytes") // "42"
					case "boolValue":
						assert.Contains(t, valueStr, "4 sensitive bytes") // "true"
					case "arrayValue":
						assert.Contains(t, valueStr, "3 sensitive entries")
					case "objectValue":
						assert.Contains(t, valueStr, "1 sensitive entries")
					}
				}
			},
		},
		{
			name: "recursive descent pattern",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"level1": map[string]interface{}{
						"password": "secret1",
						"level2": map[string]interface{}{
							"password": "secret2",
							"level3": map[string]interface{}{
								"password": "secret3",
								"other":    "public",
							},
						},
					},
					"password": "root_secret",
				},
			},
			sensitivePaths: []string{"$..password"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				// Root level password
				rootPassword := result.Object["password"].(string)
				assert.Contains(t, rootPassword, "sensitive")

				// Level 1 password
				level1 := result.Object["level1"].(map[string]interface{})
				level1Password := level1["password"].(string)
				assert.Contains(t, level1Password, "sensitive")

				// Level 2 password
				level2 := level1["level2"].(map[string]interface{})
				level2Password := level2["password"].(string)
				assert.Contains(t, level2Password, "sensitive")

				// Level 3 password
				level3 := level2["level3"].(map[string]interface{})
				level3Password := level3["password"].(string)
				assert.Contains(t, level3Password, "sensitive")

				// Other field should be unchanged
				assert.Equal(t, "public", level3["other"])
			},
		},
		{
			name: "multiple separate paths",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"database": map[string]interface{}{
							"password": "db_secret",
							"host":     "localhost",
						},
						"redis": map[string]interface{}{
							"auth": "redis_secret",
							"port": "6379",
						},
					},
					"data": map[string]interface{}{
						"api_key": "api_secret",
						"config":  "public_config",
					},
				},
			},
			sensitivePaths: []string{"spec.database.password", "spec.redis.auth", "data.api_key"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				spec := result.Object["spec"].(map[string]interface{})
				database := spec["database"].(map[string]interface{})
				redis := spec["redis"].(map[string]interface{})
				data := result.Object["data"].(map[string]interface{})

				// Sensitive fields should be redacted
				assert.Contains(t, database["password"].(string), "sensitive")
				assert.Contains(t, redis["auth"].(string), "sensitive")
				assert.Contains(t, data["api_key"].(string), "sensitive")

				// Non-sensitive fields should remain unchanged
				assert.Equal(t, "localhost", database["host"])
				assert.Equal(t, "6379", redis["port"])
				assert.Equal(t, "public_config", data["config"])
			},
		},
		{
			name: "specific array indices",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"items": []interface{}{
						"item0",
						"item1_secret",
						"item2_secret",
						"item3_secret",
						"item4",
					},
				},
			},
			sensitivePaths: []string{"items[1]", "items[2]", "items[3]"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				items := result.Object["items"].([]interface{})

				// Items 0 and 4 should be unchanged
				assert.Equal(t, "item0", items[0])
				assert.Equal(t, "item4", items[4])

				// Items 1, 2, 3 should be redacted
				for i := 1; i <= 3; i++ {
					assert.Contains(t, items[i].(string), "sensitive")
				}
			},
		},
		{
			name: "empty and nil values",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"data": map[string]interface{}{
						"empty_string": "",
						"nil_value":    nil,
						"empty_array":  []interface{}{},
						"empty_object": map[string]interface{}{},
					},
				},
			},
			sensitivePaths: []string{"data.*"},
			checkFunc: func(t *testing.T, result *unstructured.Unstructured) {
				data := result.Object["data"].(map[string]interface{})

				// All values should be redacted, even empty ones
				for key, value := range data {
					if key == "nil_value" {
						// nil values get converted to string representation
						assert.Contains(t, value.(string), "sensitive")
						assert.Contains(t, value.(string), "sensitive bytes")
					} else {
						valueStr := value.(string)
						assert.Contains(t, valueStr, "sensitive", "Key %s should be redacted", key)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resource.RedactSensitiveData(tt.input, tt.sensitivePaths)

			// Ensure original object is not modified
			assert.NotSame(t, tt.input, result, "Original object should not be modified")

			tt.checkFunc(t, result)
		})
	}
}

func TestRedactSensitiveDataEdgeCases(t *testing.T) {
	// Enable feature gate
	originalEnv := os.Getenv(featgate.FeatGateFieldSensitive.EnvVarName())
	defer func() {
		if originalEnv != "" {
			t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), originalEnv)
		}
	}()

	t.Setenv(featgate.FeatGateFieldSensitive.EnvVarName(), "true")

	tests := []struct {
		name           string
		input          *unstructured.Unstructured
		sensitivePaths []string
		expectNoChange bool
	}{
		{
			name: "non-existent path should not error",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"data": map[string]interface{}{
						"password": "secret123",
					},
				},
			},
			sensitivePaths: []string{"nonexistent.field"},
			expectNoChange: true,
		},
		{
			name: "path to non-existent array index",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"items": []interface{}{"item1", "item2"},
				},
			},
			sensitivePaths: []string{"items[10]"},
			expectNoChange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalData := tt.input.DeepCopy()
			result := resource.RedactSensitiveData(tt.input, tt.sensitivePaths)

			if tt.expectNoChange {
				// Compare the data sections to verify no changes
				assert.Equal(t, originalData.Object, result.Object, "Data should remain unchanged for invalid paths")
			}
		})
	}
}
