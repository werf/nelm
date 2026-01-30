//go:build ai_tests

package resource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/common"
)

func TestAI_ValidateResourceWithCodec(t *testing.T) {
	t.Run("valid_Deployment_passes", func(t *testing.T) {
		setupTestEnvironment(t)

		deployment := makeInstallableResource(t, map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "test-deployment",
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
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
		opts := common.ResourceValidationOptions{
			LocalResourceValidation: true,
		}

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deployment}, opts)
		assert.NoError(t, err)
	})

	t.Run("valid_ConfigMap_passes", func(t *testing.T) {
		setupTestEnvironment(t)

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
		opts := common.ResourceValidationOptions{
			LocalResourceValidation: true,
		}

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts)
		assert.NoError(t, err)
	})

	t.Run("unknown_CRD_passes", func(t *testing.T) {
		setupTestEnvironment(t)

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
		opts := common.ResourceValidationOptions{
			LocalResourceValidation: true,
		}

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{crd}, opts)
		assert.NoError(t, err)
	})

	t.Run("invalid_field_type_fails", func(t *testing.T) {
		setupTestEnvironment(t)

		pod := makeInstallableResource(t, map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
			"spec": map[string]interface{}{
				"containers": "should-be-array",
			},
		}, testReleaseNamespace)

		ctx := context.Background()
		opts := common.ResourceValidationOptions{
			LocalResourceValidation: true,
		}

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{pod}, opts)
		assertValidationError(t, err, "decode")
	})
}

func TestAI_ValidateLocal(t *testing.T) {
	t.Run("duplicate_detection", func(t *testing.T) {
		t.Run("duplicate_resources_detected", func(t *testing.T) {
			setupTestEnvironment(t)

			configMap1 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "same-configmap",
				},
				"data": map[string]interface{}{
					"key": "value1",
				},
			}, testReleaseNamespace)

			configMap2 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "same-configmap",
				},
				"data": map[string]interface{}{
					"key": "value2",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := common.ResourceValidationOptions{
				NoResourceValidation: true,
			}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap1, configMap2}, opts)
			assertValidationError(t, err, "duplicated")
		})

		t.Run("different_resources_not_duplicates", func(t *testing.T) {
			setupTestEnvironment(t)

			configMap1 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "configmap-1",
				},
				"data": map[string]interface{}{
					"key": "value1",
				},
			}, testReleaseNamespace)

			configMap2 := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "configmap-2",
				},
				"data": map[string]interface{}{
					"key": "value2",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := common.ResourceValidationOptions{
				NoResourceValidation: true,
			}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap1, configMap2}, opts)
			assert.NoError(t, err)
		})
	})

	t.Run("namespace_protection", func(t *testing.T) {
		t.Run("release_namespace_in_resources_rejected", func(t *testing.T) {
			setupTestEnvironment(t)

			namespace := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": testReleaseNamespace,
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := common.ResourceValidationOptions{
				NoResourceValidation: true,
			}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{namespace}, opts)
			assertValidationError(t, err, "cannot be deployed")
		})
	})

	t.Run("validation_skip", func(t *testing.T) {
		t.Run("skip_by_kind", func(t *testing.T) {
			setupTestEnvironment(t)

			schemas := getDefaultSchemas(t, testKubeVersion)
			server := setupSchemaServer(t, schemas)
			schemaURL := server.URL + "/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"

			invalidConfigMap := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "invalid-configmap",
				},
				"data": map[string]interface{}{
					"key": int64(123),
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})
			opts.ValidationSkip = []string{"kind=ConfigMap"}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidConfigMap}, opts)
			assert.NoError(t, err)
		})

		t.Run("skip_by_name", func(t *testing.T) {
			setupTestEnvironment(t)

			schemas := getDefaultSchemas(t, testKubeVersion)
			server := setupSchemaServer(t, schemas)
			schemaURL := server.URL + "/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"

			invalidDeployment := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "skip-me",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := makeValidationOptions(testKubeVersion, []string{schemaURL})
			opts.ValidationSkip = []string{"name=skip-me"}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidDeployment}, opts)
			assert.NoError(t, err)
		})
	})

	t.Run("integration", func(t *testing.T) {
		t.Run("LocalResourceValidation_skips_kubeconform", func(t *testing.T) {
			setupTestEnvironment(t)

			deploymentMissingSpec := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "deployment-missing-spec",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := common.ResourceValidationOptions{
				LocalResourceValidation: true,
			}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{deploymentMissingSpec}, opts)
			assert.NoError(t, err)
		})

		t.Run("NoResourceValidation_skips_all_validation", func(t *testing.T) {
			setupTestEnvironment(t)

			invalidPod := makeInstallableResource(t, map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "invalid-pod",
				},
				"spec": map[string]interface{}{
					"containers": "should-be-array",
				},
			}, testReleaseNamespace)

			ctx := context.Background()
			opts := common.ResourceValidationOptions{
				NoResourceValidation: true,
			}

			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidPod}, opts)
			assert.NoError(t, err)
		})
	})
}
