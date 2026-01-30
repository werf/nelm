//go:build ai_tests

package resource_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

const (
	testKubeVersion      = "1.30.0"
	testReleaseNamespace = "test-namespace"
)

func setupTestEnvironment(t *testing.T) {
	t.Helper()
	common.APIResourceValidationJSONSchemasCacheDir = t.TempDir()
	featgate.FeatGateResourceValidation.Enable()
}

func setupSchemaServer(t *testing.T, schemas map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		if schema, ok := schemas[path]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(schema))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	t.Cleanup(func() {
		server.Close()
	})

	return server
}

func setupLocalSchemaDir(t *testing.T, schemas map[string]string) string {
	t.Helper()

	schemaDir := t.TempDir()

	for relPath, content := range schemas {
		fullPath := filepath.Join(schemaDir, relPath)
		parentDir := filepath.Dir(fullPath)
		require.NoError(t, os.MkdirAll(parentDir, 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	return schemaDir
}

func makeInstallableResource(t *testing.T, obj map[string]interface{}, releaseNamespace string) *resource.InstallableResource {
	t.Helper()

	unstruct := &unstructured.Unstructured{Object: obj}
	resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{})

	instRes, err := resource.NewInstallableResource(resSpec, releaseNamespace, nil, resource.InstallableResourceOptions{})
	require.NoError(t, err)

	return instRes
}

func makeValidationOptions(kubeVersion string, schemaURLs []string) common.ResourceValidationOptions {
	return common.ResourceValidationOptions{
		ValidationKubeVersion:         kubeVersion,
		ValidationSchemaCacheLifetime: 1 * time.Hour,
		ValidationSchemas:             schemaURLs,
	}
}

func assertValidationError(t *testing.T, err error, expectedSubstring string) {
	t.Helper()
	require.Error(t, err)
	require.Contains(t, err.Error(), expectedSubstring)
}

type requestCountingHandler struct {
	handler      http.Handler
	requestCount *int
}

func (h *requestCountingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	*h.requestCount++
	h.handler.ServeHTTP(w, r)
}

func setupSchemaServerWithCounter(t *testing.T, schemas map[string]string) (*httptest.Server, *int) {
	t.Helper()

	requestCount := new(int)
	*requestCount = 0

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		if schema, ok := schemas[path]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(schema))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	countingHandler := &requestCountingHandler{
		handler:      baseHandler,
		requestCount: requestCount,
	}

	server := httptest.NewServer(countingHandler)

	t.Cleanup(func() {
		server.Close()
	})

	return server, requestCount
}

func getTestdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func loadSchema(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(getTestdataPath(), "schemas", name+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func getDefaultSchemas(t *testing.T, kubeVersion string) map[string]string {
	t.Helper()
	version := "v" + kubeVersion

	return map[string]string{
		version + "-standalone/deployment-apps-v1.json":            loadSchema(t, "deployment"),
		version + "-standalone/configmap-" + kubeVersion + ".json": loadSchema(t, "configmap"),
		version + "-standalone/service-" + kubeVersion + ".json":   loadSchema(t, "service"),
		version + "-standalone/pod-" + kubeVersion + ".json":       loadSchema(t, "pod"),
	}
}
