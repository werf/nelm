//go:build ai_tests

package resource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/internal/resource"
)

func TestAI_KubeConformValidator(t *testing.T) {
	t.Run("valid_resources", func(t *testing.T) {
		t.Run("valid_Deployment_passes", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": int64(3),
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": "test",
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": "test",
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "app",
									"image": "nginx:latest",
								},
							},
						},
					},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assert.NoError(t, err)
		})

		t.Run("valid_ConfigMap_passes", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-configmap",
				},
				"data": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
			assert.NoError(t, err)
		})

		t.Run("valid_Service_passes", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			service := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name": "test-service",
				},
				"spec": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     int64(80),
							"protocol": "TCP",
						},
					},
					"selector": map[string]interface{}{
						"app": "test",
					},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{service}, opts)
			assert.NoError(t, err)
		})

		t.Run("Deployment_with_extra_unknown_fields_passes", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{},
					"strategy": map[string]interface{}{
						"type": "RollingUpdate",
					},
					"minReadySeconds": int64(5),
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assert.NoError(t, err)
		})
	})

	t.Run("invalid_resources", func(t *testing.T) {
		t.Run("Deployment_missing_spec_fails", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assertValidationError(t, err, "spec")
		})

		t.Run("Deployment_replicas_as_string_fails", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": "three",
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assertValidationError(t, err, "replicas")
		})

		t.Run("Service_with_port_as_string_fails", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			service := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name": "test-service",
				},
				"spec": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     "eighty",
							"protocol": "TCP",
						},
					},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{service}, opts)
			assertValidationError(t, err, "port")
		})

		t.Run("ConfigMap_with_non_string_data_fails", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-configmap",
				},
				"data": map[string]interface{}{
					"key1": int64(123),
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
			assertValidationError(t, err, "data")
		})

		t.Run("multiple_errors_collected", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment1 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "invalid-deployment-1",
				},
			}, testReleaseNamespace)

			deployment2 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "invalid-deployment-2",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment1, deployment2}, opts)
			assertValidationError(t, err, "invalid-deployment-1")
			assertValidationError(t, err, "invalid-deployment-2")
		})
	})

	t.Run("schema_source_handling", func(t *testing.T) {
		t.Run("local_filesystem_source_works", func(t *testing.T) {
			setupTestEnvironment(t)

			schemas := getDefaultSchemas(t, testKubeVersion)
			schemaDir := setupLocalSchemaDir(t, schemas)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaDir})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assert.NoError(t, err)
		})

		t.Run("http_source_works", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
			assert.NoError(t, err)
		})

		t.Run("fallback_to_second_source", func(t *testing.T) {
			setupTestEnvironment(t)

			version := "v" + testKubeVersion
			deploymentOnlySchemas := map[string]string{
				version + "-standalone/deployment-apps-v1.json": loadSchema(t, "deployment"),
			}
			server1 := setupSchemaServer(t, deploymentOnlySchemas)
			schemaURL1 := server1.URL + schemaURLTemplate

			allSchemas := getDefaultSchemas(t, testKubeVersion)
			server2 := setupSchemaServer(t, allSchemas)
			schemaURL2 := server2.URL + schemaURLTemplate

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-configmap",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL1, schemaURL2})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
			assert.NoError(t, err)
		})

		t.Run("sanity_check_fails_no_deployment_schema", func(t *testing.T) {
			setupTestEnvironment(t)

			version := "v" + testKubeVersion
			configMapOnlySchemas := map[string]string{
				version + "-standalone/configmap-" + testKubeVersion + ".json": loadSchema(t, "configmap"),
			}
			server := setupSchemaServer(t, configMapOnlySchemas)
			schemaURL := server.URL + schemaURLTemplate

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-configmap",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
			assertValidationError(t, err, "sanity check")
		})

		t.Run("resource_without_schema_skipped", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			crd := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "custom.example.com/v1",
				"kind":       "MyCustomResource",
				"metadata": map[string]interface{}{
					"name": "test-custom-resource",
				},
				"spec": map[string]interface{}{
					"anyField": "anyValue",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{crd}, opts)
			assert.NoError(t, err)
		})
	})

	t.Run("caching", func(t *testing.T) {
		t.Run("second_validation_uses_cache", func(t *testing.T) {
			setupTestEnvironment(t)

			schemas := getDefaultSchemas(t, testKubeVersion)
			server, requestCount := setupSchemaServerWithCounter(t, schemas)
			schemaURL := server.URL + schemaURLTemplate

			deployment1 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "deployment-1",
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			deployment2 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "deployment-2",
				},
				"spec": map[string]interface{}{
					"replicas": int64(2),
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment1}, opts)
			assert.NoError(t, err)
			firstRequestCount := *requestCount

			err = resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment2}, opts)
			assert.NoError(t, err)
			secondRequestCount := *requestCount - firstRequestCount

			assert.Less(t, secondRequestCount, firstRequestCount, "second validation should use cache and make fewer requests")
		})
	})

	t.Run("edge_cases", func(t *testing.T) {
		t.Run("empty_resource_list_succeeds", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{}, opts)
			assert.NoError(t, err)
		})

		t.Run("resource_with_special_characters_in_name", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "config-with-dashes-and-123",
				},
				"data": map[string]interface{}{
					"special-key.with.dots": "value",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
			assert.NoError(t, err)
		})

		t.Run("multiple_valid_resources_pass", func(t *testing.T) {
			schemaURL := setupDefaultSchemaServer(t)

			deployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "test-deployment",
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{},
				},
			}, testReleaseNamespace)

			configMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-configmap",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			}, testReleaseNamespace)

			service := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name": "test-service",
				},
				"spec": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     int64(80),
							"protocol": "TCP",
						},
					},
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment, configMap, service}, opts)
			assert.NoError(t, err)
		})
	})
}
