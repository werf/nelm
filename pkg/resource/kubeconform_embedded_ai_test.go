//go:build ai_tests

package resource_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/schemas"
)

func TestAI_EmbeddedSchemasFallback(t *testing.T) {
	t.Run("falls_back_to_embedded_when_configured_source_has_no_schema", func(t *testing.T) {
		setupTestEnvironment(t)

		// The server answers 404 for everything, so only the embedded schemas can validate this.
		server := setupSchemaServer(t, map[string]string{})

		validDeployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{validDeployment}, opts)
		assert.NoError(t, err)
	})

	t.Run("embedded_fallback_reports_invalid_resources", func(t *testing.T) {
		setupTestEnvironment(t)

		server := setupSchemaServer(t, map[string]string{})

		object := validDeploymentObject()
		object["spec"].(map[string]interface{})["replicas"] = "should-be-integer"
		invalidDeployment := makeInstallableResource(t, object, testReleaseNamespace)

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidDeployment}, opts)
		assertValidationError(t, err, "/spec/replicas")
	})

	t.Run("configured_source_takes_precedence_over_embedded", func(t *testing.T) {
		setupTestEnvironment(t)

		// An empty schema accepts anything. The resource below omits spec.selector, which the real
		// Deployment schema requires and the codec check does not look at, so it can only pass if
		// the configured source is consulted instead of the embedded schemas.
		server := setupSchemaServer(t, map[string]string{
			"v" + testKubeVersion(t) + "-standalone/deployment-apps-v1.json": "{}",
		})

		object := validDeploymentObject()
		delete(object["spec"].(map[string]interface{}), "selector")
		invalidDeployment := makeInstallableResource(t, object, testReleaseNamespace)

		ctx := context.Background()

		// Sanity check: the embedded schemas do reject this resource.
		embeddedOnlyOpts := makeValidationOptions(nil)
		embeddedOnlyOpts.LocalResourceValidation = true

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidDeployment}, embeddedOnlyOpts)
		assertValidationError(t, err, "selector")

		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		err = resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidDeployment}, opts)
		assert.NoError(t, err)
	})

	t.Run("resource_without_any_schema_still_passes", func(t *testing.T) {
		setupTestEnvironment(t)

		server := setupSchemaServer(t, map[string]string{})

		customResource := makeInstallableResource(t, map[string]interface{}{
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
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{customResource}, opts)
		assert.NoError(t, err)
	})

	t.Run("custom_resources_validate_against_the_embedded_crd_catalog", func(t *testing.T) {
		setupTestEnvironment(t)

		ctx := context.Background()
		opts := makeValidationOptions(nil)

		// Prometheus is in the CRDs catalog, and spec.replicas is an integer there.
		valid := makeInstallableResource(t, prometheusObject(int64(2)), testReleaseNamespace)
		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{valid}, opts))

		invalid := makeInstallableResource(t, prometheusObject("should-be-integer"), testReleaseNamespace)

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalid}, opts)
		assertValidationError(t, err, "/spec/replicas")
	})

	t.Run("crd_catalog_is_consulted_next_to_the_configured_sources", func(t *testing.T) {
		setupTestEnvironment(t)

		// The native schemas come from the configured source here, but it has nothing for custom
		// resources, so the embedded catalog must still be reached for the Prometheus below.
		server := setupSchemaServer(t, kubeConformNamedSchemas(t, testKubeVersion(t)))

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		invalid := makeInstallableResource(t, prometheusObject("should-be-integer"), testReleaseNamespace)

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalid}, opts)
		assertValidationError(t, err, "/spec/replicas")
	})

	t.Run("configured_sources_are_only_requested_for_the_resources_being_validated", func(t *testing.T) {
		setupTestEnvironment(t)

		// The embedded Kubernetes schemas can always serve native resources, so nothing has to be
		// learned about the configured sources up front. They used to be probed for a Deployment schema
		// on every run, which cost a request that could not change the outcome.
		server, requestCount := setupSchemaServerWithCounter(t, map[string]string{})

		ctx := context.Background()
		opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

		configMap := makeInstallableResource(t, validConfigMapObject(), testReleaseNamespace)
		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{configMap}, opts))

		// Exactly one request, the schema lookup for the ConfigMap, and no probe on top of it.
		assert.Equal(t, 1, *requestCount, "the configured sources must not be probed")
	})

	t.Run("unreachable_configured_source_still_fails_the_run", func(t *testing.T) {
		setupTestEnvironment(t)

		// Falling back to the embedded schemas on a broken configured source would hide the mistake,
		// so an unreachable source is deliberately fatal even though the embedded schemas could have
		// validated this resource.
		validDeployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

		ctx := context.Background()
		opts := makeValidationOptions([]string{"http://127.0.0.1:1/{{ .ResourceKind }}{{ .KindSuffix }}.json"})

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{validDeployment}, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed downloading schema")
	})
}

func TestAI_EmbeddedSchemasNotUnpackedWhenNotNeeded(t *testing.T) {
	setupTestEnvironment(t)

	// The configured source has every schema this resource needs, so the embedded bundle must not
	// be unpacked at all.
	schemas := getDefaultSchemas(t, testKubeVersion(t))
	server := setupSchemaServer(t, schemas)

	validDeployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

	ctx := context.Background()
	opts := makeValidationOptions([]string{server.URL + schemaURLTemplate})

	require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{validDeployment}, opts))

	assert.False(t, embeddedSchemasUnpacked(t), "embedded schemas must stay packed while the configured sources suffice")
}

func TestAI_EmbeddedSchemasOnly(t *testing.T) {
	t.Run("ignores_configured_sources", func(t *testing.T) {
		setupTestEnvironment(t)

		validDeployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

		ctx := context.Background()

		// Nothing is listening on this port: if the configured sources were consulted at all, looking
		// the Deployment schema up would fail the run.
		opts := makeValidationOptions([]string{"http://127.0.0.1:1/{{ .ResourceKind }}{{ .KindSuffix }}.json"})
		opts.LocalResourceValidation = true

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{validDeployment}, opts)
		assert.NoError(t, err)
	})

	t.Run("validates_invalid_resources", func(t *testing.T) {
		setupTestEnvironment(t)

		object := validDeploymentObject()
		object["spec"].(map[string]interface{})["template"] = "should-be-object"
		invalidDeployment := makeInstallableResource(t, object, testReleaseNamespace)

		ctx := context.Background()
		opts := common.ResourceValidationOptions{LocalResourceValidation: true}

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalidDeployment}, opts)
		assertValidationError(t, err, "/spec/template")
	})

	t.Run("collects_the_schemas_of_several_kube_versions", func(t *testing.T) {
		embedded, err := schemas.KubeVersions()
		require.NoError(t, err)
		require.Len(t, embedded, 5, "five Kubernetes minor versions are expected to be collected")
		assert.Equal(t, embedded[0], testKubeVersion(t),
			"the newest collected version must be the one resources are validated against")
	})

	t.Run("validates_resources_of_api_versions_dropped_by_newer_kube_versions", func(t *testing.T) {
		setupTestEnvironment(t)

		// PodCertificateRequest certificates.k8s.io/v1alpha1 exists in Kubernetes 1.34 alone, so its
		// schema can only come from an older collected version and only while 1.34 is one of them.
		// Replace it with another resource of a dropped API version once the window moves past 1.34.
		//
		// The path in the error is what tells the two validators apart: the schema reports
		// "/spec/maxExpirationSeconds", the client-go codec would report "spec.maxExpirationSeconds".
		ctx := context.Background()
		opts := common.ResourceValidationOptions{LocalResourceValidation: true}

		valid := makeInstallableResource(t, podCertificateRequestObject(int64(3600)), testReleaseNamespace)
		require.NoError(t, resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{valid}, opts))

		invalid := makeInstallableResource(t, podCertificateRequestObject("should-be-integer"), testReleaseNamespace)

		err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{invalid}, opts)
		assertValidationError(t, err, "/spec/maxExpirationSeconds")
	})

	t.Run("unpacks_schemas_only_once", func(t *testing.T) {
		setupTestEnvironment(t)

		validDeployment := makeInstallableResource(t, validDeploymentObject(), testReleaseNamespace)

		ctx := context.Background()
		opts := common.ResourceValidationOptions{LocalResourceValidation: true}

		for range 2 {
			err := resource.ValidateLocal(ctx, testReleaseNamespace, []*resource.InstallableResource{validDeployment}, opts)
			require.NoError(t, err)
		}
	})
}

func embeddedSchemasUnpacked(t *testing.T) bool {
	t.Helper()

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	require.NotEmpty(t, cacheHome, "setupTestEnvironment must redirect the cache dir")

	entries, err := os.ReadDir(filepath.Join(cacheHome, "helm", common.CacheDirEmbeddedAPIResourceJSONSchemas))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}
	}

	return false
}

// podCertificateRequestObject builds a resource of an API version that only the older collected
// Kubernetes versions have a schema for.
func podCertificateRequestObject(maxExpirationSeconds interface{}) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "certificates.k8s.io/v1alpha1",
		"kind":       "PodCertificateRequest",
		"metadata": map[string]interface{}{
			"name": "test-pod-certificate-request",
		},
		"spec": map[string]interface{}{
			"maxExpirationSeconds": maxExpirationSeconds,
			"nodeName":             "test-node",
			"nodeUID":              "11111111-1111-1111-1111-111111111111",
			"pkixPublicKey":        "dGVzdC1wdWJsaWMta2V5",
			"podName":              "test-pod",
			"podUID":               "22222222-2222-2222-2222-222222222222",
			"proofOfPossession":    "dGVzdC1wcm9vZg==",
			"serviceAccountName":   "test-service-account",
			"serviceAccountUID":    "33333333-3333-3333-3333-333333333333",
			"signerName":           "example.com/test-signer",
		},
	}
}

func prometheusObject(replicas interface{}) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "Prometheus",
		"metadata": map[string]interface{}{
			"name": "test-prometheus",
		},
		"spec": map[string]interface{}{
			"replicas": replicas,
		},
	}
}

func validDeploymentObject() map[string]interface{} {
	return map[string]interface{}{
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
	}
}
