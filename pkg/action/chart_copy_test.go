package action

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/werf/nelm/v2/pkg/chartyaml"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

func TestChartCopy_ArchiveCanceled(t *testing.T) {
	chartData := buildChartCopyTestChart(t, map[string]string{"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\n"})
	archivePath := writeWerfChartCopyTestArchive(t, chartData, nil)

	ctx, cancel := context.WithCancel(newChartPackTestContext())
	cancel()

	err := ChartCopy(ctx, "archive:"+archivePath, filepath.Join(t.TempDir(), "copy.tar"), newChartCopyTestOptions(t))
	require.ErrorIs(t, err, context.Canceled)
}

func TestChartCopy_ArchiveExpandedSizeLimit(t *testing.T) {
	layer := tarChartCopyTestFiles(t, map[string][]byte{"zeros": make([]byte, 1<<20)})
	imageTarball := tarChartCopyTestFiles(t, map[string][]byte{
		"manifest.json": []byte(`[{"Config":"config.json","Layers":["layer.tar"]}]`),
		"config.json":   []byte(`{"architecture":"amd64","os":"linux"}`),
		"layer.tar":     layer,
	})
	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: old.registry/project:abc\n",
	})
	archivePath := writeWerfChartCopyTestArchive(t, chartData, map[string][]byte{"abc": imageTarball})

	tests := []struct {
		name        string
		maxSize     int64
		expectedErr string
	}{
		{
			name:        "outer archive",
			maxSize:     1 << 10,
			expectedErr: "archive expands to more than 1024 bytes",
		},
		{
			name:        "nested image tarball",
			maxSize:     256 << 10,
			expectedErr: "archive expands to more than 262144 bytes",
		},
		{
			name:    "within the limit",
			maxSize: 4 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newChartCopyTestOptions(t)
			opts.ArchiveMaxExpandedSize = tt.maxSize

			err := ChartCopy(newChartPackTestContext(), "archive:"+archivePath, filepath.Join(t.TempDir(), "copy.tar"), opts)
			if tt.expectedErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestChartCopy_ArchiveRoundTrip(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	backend := pushChartCopyTestImage(t, host+"/src/images", "backend", "amd64", "arm64")
	frontend := pushChartCopyTestImage(t, host+"/src/images", "frontend", "amd64")

	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": fmt.Sprintf("apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s\n    - name: frontend\n      image: %s\n", backend.Ref, frontend.Ref),
	})
	provData := signChartCopyTestChart(t, chartData)
	_, err := client.Push(chartData, host+"/src/app:1.0.0", registry.PushOptProvData(provData))
	require.NoError(t, err)

	for i, archiveAddr := range []string{"chart.tar", "chart.tar.gz", "archive:"} {
		t.Run(archiveAddr, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), strings.TrimPrefix(archiveAddr, "archive:"))
			if strings.HasPrefix(archiveAddr, "archive:") {
				archivePath = filepath.Join(archivePath, "bundle")
			}

			archive := lo.Ternary(strings.HasPrefix(archiveAddr, "archive:"), "archive:"+archivePath, archivePath)

			require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", archive, newChartCopyTestOptions(t)))

			index := readChartCopyTestArchiveIndex(t, archivePath)
			entries := lo.Map(index.Manifests, func(desc ocispec.Descriptor, _ int) string {
				return fmt.Sprintf("%s %s %s", desc.Annotations[chartCopyArchiveEntryAnnotation], desc.Annotations[ocispec.AnnotationRefName], lo.Ternary(desc.Annotations[chartCopyArchiveEntryAnnotation] == chartCopyArchiveEntryChart, "", desc.Digest.String()))
			})
			assert.ElementsMatch(t, []string{
				"chart 1.0.0 ",
				fmt.Sprintf("image %s %s", backend.Ref, backend.Desc.Digest),
				fmt.Sprintf("image %s %s", frontend.Ref, frontend.Desc.Digest),
			}, entries)

			dst := fmt.Sprintf("%s/roundtrip%d/app", host, i)
			require.NoError(t, ChartCopy(newChartPackTestContext(), archive, "oci://"+dst, newChartCopyTestOptions(t)))

			requireChartCopyTestImage(t, dst, "backend", backend)
			requireChartCopyTestImage(t, dst, "frontend", frontend)

			result, err := client.Pull(dst+":1.0.0", registry.PullOptWithProv(true), registry.PullOptIgnoreMissingProv(true))
			require.NoError(t, err)
			assert.Empty(t, result.Prov.Data)
			assert.Equal(t, fmt.Sprintf("apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s:backend@%s\n    - name: frontend\n      image: %s:frontend@%s\n", dst, backend.Desc.Digest, dst, frontend.Desc.Digest), readChartCopyTestChartFiles(t, result.Chart.Data)["app/Chart.yaml"])

			copyArchivePath := filepath.Join(t.TempDir(), "copy.tar")
			require.NoError(t, ChartCopy(newChartPackTestContext(), archive, copyArchivePath, newChartCopyTestOptions(t)))
			assert.ElementsMatch(t, index.Manifests, readChartCopyTestArchiveIndex(t, copyArchivePath).Manifests)
		})
	}
}

func TestChartCopy_BasicAuthOnlyForChartHosts(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	var (
		mu                   sync.Mutex
		hostileAuthorization []string
	)

	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()

		hostileAuthorization = append(hostileAuthorization, r.Header.Get("Authorization"))
		mu.Unlock()

		w.Header().Set("WWW-Authenticate", `Basic realm="hostile"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(hostile.Close)

	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: " + strings.TrimPrefix(hostile.URL, "http://") + "/images:backend\n",
	})
	_, err := client.Push(chartData, host+"/src/app:1.0.0")
	require.NoError(t, err)

	opts := newChartCopyTestOptions(t)
	opts.ChartRepoBasicAuthUsername = "user"
	opts.ChartRepoBasicAuthPassword = "secret"

	require.Error(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", "oci://"+host+"/dst/app", opts))

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, hostileAuthorization)

	for _, authorization := range hostileAuthorization {
		assert.NotContains(t, authorization, base64.StdEncoding.EncodeToString([]byte("user:secret")))
	}
}

func TestChartCopy_DependencyImages(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	depImage := pushChartCopyTestImage(t, host+"/src/images", "dep", "amd64", "arm64")
	nestedImage := pushChartCopyTestImage(t, host+"/src/images", "nested", "amd64")

	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml":                          "apiVersion: v2\nname: app\nversion: 1.0.0\n",
		"charts/dep/Chart.yaml":               "apiVersion: v2\nname: dep\nversion: 0.1.0\nannotations:\n  helm.sh/images: |\n    - name: dep\n      image: " + depImage.Ref + "\n",
		"charts/dep/charts/nested/Chart.yaml": "apiVersion: v2\nname: nested\nversion: 0.1.0\nannotations:\n  helm.sh/images: |\n    - name: nested\n      image: " + nestedImage.Ref + "\n    - name: dep\n      image: " + depImage.Ref + "\n",
	})
	_, err := client.Push(chartData, host+"/src/app:1.0.0")
	require.NoError(t, err)

	archivePath := filepath.Join(t.TempDir(), "chart.tar")
	require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", archivePath, newChartCopyTestOptions(t)))

	var rewriteMapping map[string]string

	opts := newChartCopyTestOptions(t)
	opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
		rewriteMapping = imageMapping

		return nil
	}

	dst := host + "/dst/app"
	require.NoError(t, ChartCopy(newChartPackTestContext(), archivePath, "oci://"+dst, opts))

	requireChartCopyTestImage(t, dst, "dep", depImage)
	requireChartCopyTestImage(t, dst, "nested", nestedImage)

	newDepRef := fmt.Sprintf("%s:dep@%s", dst, depImage.Desc.Digest)
	newNestedRef := fmt.Sprintf("%s:nested@%s", dst, nestedImage.Desc.Digest)
	assert.Equal(t, map[string]string{depImage.Ref: newDepRef, nestedImage.Ref: newNestedRef}, rewriteMapping)

	result, err := client.Pull(dst + ":1.0.0")
	require.NoError(t, err)

	files := readChartCopyTestChartFiles(t, result.Chart.Data)
	assert.Contains(t, files["app/charts/dep/Chart.yaml"], "image: "+newDepRef+"\n")
	assert.NotContains(t, files["app/charts/dep/Chart.yaml"], depImage.Ref)
	assert.Contains(t, files["app/charts/dep/charts/nested/Chart.yaml"], "image: "+newNestedRef+"\n")
	assert.Contains(t, files["app/charts/dep/charts/nested/Chart.yaml"], "image: "+newDepRef+"\n")
}

func TestChartCopy_NonDistributableLayers(t *testing.T) {
	const nonDistributableMediaType = "application/vnd.oci.image.layer.nondistributable.v1.tar+gzip"

	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	tests := []struct {
		name        string
		served      bool
		expectedErr string
	}{
		{
			name:   "served by the source",
			served: true,
		},
		{
			name:        "not served by the source",
			expectedErr: "the source does not serve blob",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			srcImages := fmt.Sprintf("%s/src%d/images", host, i)
			repository := newChartCopyTestRepository(t, srcImages)

			configDesc, err := oras.PushBytes(ctx, repository, ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64","os":"windows"}`))
			require.NoError(t, err)

			layerData := gzipChartCopyTestData(t, []byte(fmt.Sprintf("foreign layer %d", i)))
			layerDesc := content.NewDescriptorFromBytes(nonDistributableMediaType, layerData)
			layerDesc.URLs = []string{"https://example.com/layer"}

			if tt.served {
				require.NoError(t, repository.Push(ctx, layerDesc, bytes.NewReader(layerData)))
			}

			manifestData, err := json.Marshal(ocispec.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: ocispec.MediaTypeImageManifest,
				Config:    configDesc,
				Layers:    []ocispec.Descriptor{layerDesc},
			})
			require.NoError(t, err)

			manifestDesc, err := oras.TagBytes(ctx, repository, ocispec.MediaTypeImageManifest, manifestData, "windows")
			require.NoError(t, err)

			imageRef := fmt.Sprintf("%s:windows@%s", srcImages, manifestDesc.Digest)
			chartData := buildChartCopyTestChart(t, map[string]string{
				"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: windows\n      image: " + imageRef + "\n",
			})
			src := fmt.Sprintf("%s/src%d/app", host, i)
			_, err = client.Push(chartData, src+":1.0.0")
			require.NoError(t, err)

			dst := fmt.Sprintf("%s/dst%d/app", host, i)

			err = ChartCopy(newChartPackTestContext(), "oci://"+src+":1.0.0", "oci://"+dst, newChartCopyTestOptions(t))
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
				assert.ErrorContains(t, err, layerDesc.Digest.String())

				return
			}

			require.NoError(t, err)

			exists, err := newChartCopyTestRepository(t, dst).Exists(ctx, layerDesc)
			require.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

func TestChartCopy_Provenance(t *testing.T) {
	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)

	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	chartData := buildChartCopyTestChart(t, map[string]string{"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\n"})
	provData := signChartCopyTestChart(t, chartData)
	_, err = client.Push(chartData, host+"/src/app:1.0.0", registry.PushOptProvData(provData))
	require.NoError(t, err)

	tests := []struct {
		name          string
		to            string
		sign          bool
		rewrite       bool
		expectedChart []byte
		expectedProv  string
	}{
		{
			name:          "unchanged chart keeps the provenance",
			to:            host + "/unchanged/app",
			expectedChart: chartData,
			expectedProv:  "source",
		},
		{
			name:         "chart changed by the rewrite hook drops the provenance",
			to:           host + "/rewritten/app",
			rewrite:      true,
			expectedProv: "none",
		},
		{
			name:         "changed chart drops the provenance",
			to:           host + "/changed/renamed",
			expectedProv: "none",
		},
		{
			name:         "changed chart is signed with the given key",
			to:           host + "/signed/renamed",
			sign:         true,
			expectedProv: "new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newChartCopyTestOptions(t)
			opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
				if tt.rewrite {
					c := chart.(*v2chart.Chart)
					c.WerfDataFiles = append(c.WerfDataFiles, &chartcommon.File{Name: ".werf-data/extra-values.yaml", Data: []byte("werf:\n  repo: " + repo + "\n")})
				}

				return nil
			}

			if tt.sign {
				opts.Sign = true
				opts.SignKey = "helm-test"
				opts.SignKeyring = keyringPath
			}

			require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", "oci://"+tt.to, opts))

			result, err := client.Pull(tt.to+":1.0.0", registry.PullOptWithProv(true), registry.PullOptIgnoreMissingProv(true))
			require.NoError(t, err)

			if tt.expectedChart != nil {
				assert.Equal(t, tt.expectedChart, result.Chart.Data)
			}

			if tt.rewrite {
				assert.Equal(t, "werf:\n  repo: "+tt.to+"\n", readChartCopyTestChartFiles(t, result.Chart.Data)["app/.werf-data/extra-values.yaml"])
			}

			switch tt.expectedProv {
			case "source":
				assert.Equal(t, provData, result.Prov.Data)
			case "none":
				assert.Empty(t, result.Prov.Data)
			case "new":
				require.NotEmpty(t, result.Prov.Data)
				assert.NotEqual(t, provData, result.Prov.Data)
				verifyChartCopyTestSignature(t, result.Chart.Data, result.Prov.Data, "renamed-1.0.0.tgz")
			}
		})
	}
}

func TestChartCopy_RegistryToRegistry(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	backend := pushChartCopyTestImage(t, host+"/src/images", "backend", "amd64", "arm64")
	frontend := pushChartCopyTestImage(t, host+"/src/images", "frontend", "amd64")
	nginx := pushChartCopyTestImage(t, host+"/library/nginx", "1.25", "amd64")
	nginxRef := host + "/library/nginx:1.25"

	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml":                   fmt.Sprintf("apiVersion: v2\n# keep me\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s\n    - name: frontend\n      image: %s\n    - name: nginx\n      image: %s\n", backend.Ref, frontend.Ref, nginxRef),
		".werf-data/extra-values.yaml": "werf:\n  image:\n    backend: " + backend.Ref + "\n",
	})
	_, err := client.Push(chartData, host+"/src/app:1.0.0")
	require.NoError(t, err)

	var (
		rewriteRepo    string
		rewriteMapping map[string]string
	)

	opts := newChartCopyTestOptions(t)
	opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
		rewriteRepo, rewriteMapping = repo, imageMapping

		for _, f := range chart.(*v2chart.Chart).WerfDataFiles {
			f.Data = []byte(strings.ReplaceAll(string(f.Data), backend.Ref, imageMapping[backend.Ref]))
		}

		return nil
	}

	require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", "oci://"+host+"/dst/myapp:v1.2.3", opts))

	dst := host + "/dst/myapp"
	expectedMapping := map[string]string{
		backend.Ref:  fmt.Sprintf("%s:backend@%s", dst, backend.Desc.Digest),
		frontend.Ref: fmt.Sprintf("%s:frontend@%s", dst, frontend.Desc.Digest),
		nginxRef:     fmt.Sprintf("%s:1.25@%s", dst, nginx.Desc.Digest),
	}
	assert.Equal(t, dst, rewriteRepo)
	assert.Equal(t, expectedMapping, rewriteMapping)

	requireChartCopyTestImage(t, dst, "backend", backend)
	requireChartCopyTestImage(t, dst, "frontend", frontend)
	requireChartCopyTestImage(t, dst, "1.25", nginx)

	for _, tag := range []string{"1.2.3", "v1.2.3"} {
		result, err := client.Pull(dst + ":" + tag)
		require.NoError(t, err, tag)

		files := readChartCopyTestChartFiles(t, result.Chart.Data)
		assert.Equal(t, fmt.Sprintf("apiVersion: v2\n# keep me\nname: myapp\nversion: 1.2.3\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s\n    - name: frontend\n      image: %s\n    - name: nginx\n      image: %s\n", expectedMapping[backend.Ref], expectedMapping[frontend.Ref], expectedMapping[nginxRef]), files["myapp/Chart.yaml"], tag)
		assert.Equal(t, "werf:\n  image:\n    backend: "+expectedMapping[backend.Ref]+"\n", files["myapp/.werf-data/extra-values.yaml"], tag)
	}
}

func TestChartCopy_RequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	opts := newChartCopyTestOptions(t)
	opts.ChartRepoRequestTimeout = 100 * time.Millisecond

	err := ChartCopy(newChartPackTestContext(), "oci://"+strings.TrimPrefix(server.URL, "http://")+"/app:1.0.0", filepath.Join(t.TempDir(), "chart.tar"), opts)
	require.ErrorContains(t, err, "Client.Timeout exceeded")
}

func TestChartCopy_RewriteChartMetadataAndValues(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml":            "apiVersion: v2\n# keep me\nname: app\nversion: 1.0.0\nannotations:\n  team: payments\n  obsolete: \"yes\"\n",
		"values.yaml":           "# comment\nwerf:\n  image:\n    backend: old.registry/project:abc\n",
		"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
	})
	_, err := client.Push(chartData, host+"/src/app:1.0.0")
	require.NoError(t, err)

	tests := []struct {
		name          string
		rewrite       func(chart *v2chart.Chart)
		expectedFiles map[string]string
		expectedErr   string
	}{
		{
			name: "annotations",
			rewrite: func(chart *v2chart.Chart) {
				chart.Metadata.Annotations["team"] = "billing"
				chart.Metadata.Annotations["werf.io/version"] = "v3"
				delete(chart.Metadata.Annotations, "obsolete")
			},
			expectedFiles: map[string]string{
				"app/Chart.yaml": "apiVersion: v2\n# keep me\nname: app\nversion: 1.0.0\nannotations:\n  team: billing\n  werf.io/version: v3\n",
			},
		},
		{
			name: "values",
			rewrite: func(chart *v2chart.Chart) {
				chart.Values["werf"].(map[string]any)["image"].(map[string]any)["backend"] = "new.registry/project:abc"
				chart.Dependencies()[0].Values = map[string]any{"dep": "value"}
			},
			expectedFiles: map[string]string{
				"app/values.yaml":            "werf:\n  image:\n    backend: new.registry/project:abc\n",
				"app/charts/dep/values.yaml": "dep: value\n",
			},
		},
		{
			name: "values changed both raw and parsed keep the raw file",
			rewrite: func(chart *v2chart.Chart) {
				chart.Values["werf"] = "parsed"
				for _, f := range chart.Raw {
					if f.Name == "values.yaml" {
						f.Data = []byte("# raw\nwerf: raw\n")
					}
				}
			},
			expectedFiles: map[string]string{
				"app/values.yaml": "# raw\nwerf: raw\n",
			},
		},
		{
			name: "other metadata",
			rewrite: func(chart *v2chart.Chart) {
				chart.Metadata.Description = "changed"
			},
			expectedErr: "RewriteChart changed Chart.yaml fields other than annotations",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newChartCopyTestOptions(t)
			opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
				tt.rewrite(chart.(*v2chart.Chart))

				return nil
			}

			dst := fmt.Sprintf("%s/dst%d/app", host, i)

			err := ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", "oci://"+dst, opts)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)

				return
			}

			require.NoError(t, err)

			result, err := client.Pull(dst + ":1.0.0")
			require.NoError(t, err)

			files := readChartCopyTestChartFiles(t, result.Chart.Data)
			for name, expected := range tt.expectedFiles {
				assert.Equal(t, expected, files[name], name)
			}
		})
	}
}

func TestChartCopy_TagCollisions(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	first := pushChartCopyTestImage(t, host+"/first/images", "backend", "amd64")
	second := pushChartCopyTestImage(t, host+"/second/images", "backend", "arm64")
	sameDigest := pushChartCopyTestImage(t, host+"/first/images", "same", "amd64")
	versionTagged := pushChartCopyTestImage(t, host+"/first/images", "1.0.0", "amd64")
	extraTagged := pushChartCopyTestImage(t, host+"/first/images", "latest", "amd64")

	tests := []struct {
		name        string
		images      []string
		expectedErr string
	}{
		{
			name:        "images with the same tag and different digests",
			images:      []string{first.Ref, second.Ref},
			expectedErr: fmt.Sprintf(`tag "backend" would be used for both image %q and image %q`, first.Ref, second.Ref),
		},
		{
			name:        "image tag is the chart version tag",
			images:      []string{versionTagged.Ref},
			expectedErr: fmt.Sprintf(`tag "1.0.0" would be used for both the chart and image %q`, versionTagged.Ref),
		},
		{
			name:        "image tag is the chart extra tag",
			images:      []string{extraTagged.Ref},
			expectedErr: fmt.Sprintf(`tag "latest" would be used for both the chart and image %q`, extraTagged.Ref),
		},
		{
			name:   "images with the same tag and digest",
			images: []string{first.Ref, host + "/first/images:backend@" + first.Desc.Digest.String(), sameDigest.Ref},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := lo.Map(tt.images, func(image string, j int) string {
				return fmt.Sprintf("    - name: image%d\n      image: %s\n", j, image)
			})
			chartData := buildChartCopyTestChart(t, map[string]string{
				"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n" + strings.Join(images, ""),
			})

			src := fmt.Sprintf("%s/src%d/app", host, i)
			_, err := client.Push(chartData, src+":1.0.0", registry.PushOptExtraTags([]string{"latest"}))
			require.NoError(t, err)

			dst := fmt.Sprintf("%s/dst%d/app", host, i)

			err = ChartCopy(newChartPackTestContext(), "oci://"+src+":latest", "oci://"+dst, newChartCopyTestOptions(t))
			if tt.expectedErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, tt.expectedErr)

			var pushedTags []string

			_ = newChartCopyTestRepository(t, dst).Tags(context.Background(), "", func(tags []string) error {
				pushedTags = append(pushedTags, tags...)

				return nil
			})
			assert.Empty(t, pushedTags)
		})
	}
}

func TestChartCopy_Tags(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	chartData := buildChartCopyTestChart(t, map[string]string{"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\n"})
	_, err := client.Push(chartData, host+"/src/app:1.0.0", registry.PushOptExtraTags([]string{"latest"}))
	require.NoError(t, err)

	tests := []struct {
		name            string
		to              string
		expectedVersion string
		expectedTags    []string
	}{
		{
			name:            "without tag keeps the source version and tag",
			to:              "oci://" + host + "/notag/app",
			expectedVersion: "^1\\.0\\.0$",
			expectedTags:    []string{"1.0.0", "latest"},
		},
		{
			name:            "semver tag with v prefix",
			to:              "oci://" + host + "/semver/app:v2.0.0",
			expectedVersion: "^2\\.0\\.0$",
			expectedTags:    []string{"2.0.0", "v2.0.0"},
		},
		{
			name:            "semver tag with prerelease",
			to:              host + "/prerelease/app:1.2.3-rc.1",
			expectedVersion: "^1\\.2\\.3-rc\\.1$",
			expectedTags:    []string{"1.2.3-rc.1"},
		},
		{
			name:            "non-semver tag",
			to:              "oci://" + host + "/nonsemver/app:main",
			expectedVersion: "^0\\.0\\.0-[0-9]+-main$",
			expectedTags:    []string{"main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app", tt.to, newChartCopyTestOptions(t)))

			repo := strings.TrimPrefix(tt.to, "oci://")
			repo = repo[:strings.LastIndex(repo, "/")] + "/app"

			for _, tag := range tt.expectedTags {
				result, err := client.Pull(repo + ":" + tag)
				require.NoError(t, err, tag)
				assert.Regexp(t, tt.expectedVersion, result.Chart.Meta.Version, tag)

				versionTag := strings.ReplaceAll(result.Chart.Meta.Version, "+", "_")
				versionDesc, err := client.Resolve(repo + ":" + versionTag)
				require.NoError(t, err, versionTag)
				assert.Equal(t, result.Manifest.Digest, versionDesc.Digest.String(), tag)
			}
		})
	}
}

func TestChartCopy_WerfArchive(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	configData := []byte(`{"architecture":"amd64","os":"linux"}`)
	compressedLayer := gzipChartCopyTestData(t, []byte("compressed layer"))
	configName := digest.FromBytes(configData).String()
	compressedLayerName := digest.FromBytes(compressedLayer).Encoded() + ".tar.gz"

	imageTarball := tarChartCopyTestFiles(t, map[string][]byte{
		"manifest.json":     []byte(fmt.Sprintf(`[{"Config":%q,"RepoTags":["old.registry/project:abc"],"Layers":[%q,"plain/layer.tar"]}]`, configName, compressedLayerName)),
		configName:          configData,
		compressedLayerName: compressedLayer,
		"plain/layer.tar":   tarChartCopyTestFiles(t, map[string][]byte{"file": []byte("plain layer")}),
	})

	sourceRef := "old.registry/project:abc@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: project\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: " + sourceRef + "\n",
	})

	archivePath := writeWerfChartCopyTestArchive(t, chartData, map[string][]byte{"abc": imageTarball})

	var rewriteMapping map[string]string

	opts := newChartCopyTestOptions(t)
	opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
		rewriteMapping = imageMapping

		return nil
	}

	dst := host + "/werf/app"
	require.NoError(t, ChartCopy(newChartPackTestContext(), "archive:"+archivePath, "oci://"+dst+":2.0.0", opts))

	ctx := context.Background()
	repository := newChartCopyTestRepository(t, dst)

	manifestDesc, err := repository.Resolve(ctx, "abc")
	require.NoError(t, err)
	assert.Equal(t, dockerManifestMediaType, manifestDesc.MediaType)
	assert.Equal(t, map[string]string{sourceRef: fmt.Sprintf("%s:abc@%s", dst, manifestDesc.Digest)}, rewriteMapping)

	manifestData, err := content.FetchAll(ctx, repository, manifestDesc)
	require.NoError(t, err)

	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	assert.Equal(t, digest.FromBytes(configData), manifest.Config.Digest)
	require.Len(t, manifest.Layers, 2)
	assert.Equal(t, digest.FromBytes(compressedLayer), manifest.Layers[0].Digest)

	plainLayer, err := content.FetchAll(ctx, repository, manifest.Layers[1])
	require.NoError(t, err)

	gzipReader, err := gzip.NewReader(bytes.NewReader(plainLayer))
	require.NoError(t, err)
	plainLayerData, err := io.ReadAll(gzipReader)
	require.NoError(t, err)
	assert.Equal(t, tarChartCopyTestFiles(t, map[string][]byte{"file": []byte("plain layer")}), plainLayerData)

	result, err := client.Pull(dst + ":2.0.0")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("apiVersion: v2\nname: app\nversion: 2.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s:abc@%s\n", dst, manifestDesc.Digest), readChartCopyTestChartFiles(t, result.Chart.Data)["app/Chart.yaml"])
}

func TestChartCopy_WerfArchiveDuplicateMembers(t *testing.T) {
	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: old.registry/project:abc\n",
	})
	manifest := []byte(`[{"Config":"config.json","Layers":["layer.tar"]}]`)
	config := []byte(`{"architecture":"amd64","os":"linux"}`)
	layer := tarChartCopyTestFiles(t, map[string][]byte{"file": []byte("data")})

	tests := []struct {
		name        string
		entries     []chartCopyTestTarEntry
		expectedErr string
	}{
		{
			name: "outer archive",
			entries: []chartCopyTestTarEntry{
				{Name: werfArchiveChartFileName, Data: chartData},
				{Name: "./" + werfArchiveChartFileName, Data: chartData},
			},
			expectedErr: `archive has duplicate member "chart.tar.gz"`,
		},
		{
			name: "image tarball",
			entries: []chartCopyTestTarEntry{
				{Name: werfArchiveChartFileName, Data: chartData},
				{Name: "images/abc.tar.gz", Data: gzipChartCopyTestData(t, tarChartCopyTestEntries(t, []chartCopyTestTarEntry{
					{Name: "manifest.json", Data: manifest},
					{Name: "manifest.json", Data: manifest},
					{Name: "config.json", Data: config},
					{Name: "layer.tar", Data: layer},
				}))},
			},
			expectedErr: `image tarball has duplicate member "manifest.json"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
			require.NoError(t, os.WriteFile(archivePath, gzipChartCopyTestData(t, tarChartCopyTestEntries(t, tt.entries)), 0o644))

			err := ChartCopy(newChartPackTestContext(), "archive:"+archivePath, filepath.Join(t.TempDir(), "copy.tar"), newChartCopyTestOptions(t))
			require.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestChartCopy_WerfArchiveUnsupportedLayers(t *testing.T) {
	tests := []struct {
		name        string
		layer       []byte
		expectedErr string
	}{
		{
			name:        "zstd",
			layer:       append([]byte{0x28, 0xb5, 0x2f, 0xfd}, []byte("zstd data")...),
			expectedErr: `layer "layer" is zstd-compressed, only gzip-compressed and uncompressed tar layers are supported`,
		},
		{
			name:        "not a tar",
			layer:       []byte("not a tar"),
			expectedErr: `layer "layer" is neither a gzip-compressed nor an uncompressed tar`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageTarball := tarChartCopyTestFiles(t, map[string][]byte{
				"manifest.json": []byte(`[{"Config":"config.json","Layers":["layer"]}]`),
				"config.json":   []byte(`{"architecture":"amd64","os":"linux"}`),
				"layer":         tt.layer,
			})
			chartData := buildChartCopyTestChart(t, map[string]string{
				"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: old.registry/project:abc\n",
			})
			archivePath := writeWerfChartCopyTestArchive(t, chartData, map[string][]byte{"abc": imageTarball})

			err := ChartCopy(newChartPackTestContext(), "archive:"+archivePath, filepath.Join(t.TempDir(), "copy.tar"), newChartCopyTestOptions(t))
			require.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestChartCopy_WerfArchiveWithImagesOption(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	configData := []byte(`{"architecture":"amd64","os":"linux"}`)
	layer := gzipChartCopyTestData(t, []byte("layer"))
	imageTarball := tarChartCopyTestFiles(t, map[string][]byte{
		"manifest.json": []byte(fmt.Sprintf(`[{"Config":"config.json","Layers":["%s.tar.gz"]}]`, digest.FromBytes(layer).Encoded())),
		"config.json":   configData,
		digest.FromBytes(layer).Encoded() + ".tar.gz": layer,
	})

	sourceRef := "old.registry/project:abc"
	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml":  "apiVersion: v2\nname: project\nversion: 1.0.0\n",
		"values.yaml": "werf:\n  image:\n    backend: " + sourceRef + "\n",
	})
	archivePath := writeWerfChartCopyTestArchive(t, chartData, map[string][]byte{"abc": imageTarball})

	var rewriteMapping map[string]string

	opts := newChartCopyTestOptions(t)
	opts.Images = []chartyaml.Image{{Name: "backend", Image: sourceRef}, {Name: "backend-copy", Image: sourceRef}}
	opts.RewriteChart = func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error {
		rewriteMapping = imageMapping

		return nil
	}

	dst := host + "/werf/app"
	require.NoError(t, ChartCopy(newChartPackTestContext(), "archive:"+archivePath, "oci://"+dst+":2.0.0", opts))

	imageDesc, err := newChartCopyTestRepository(t, dst).Resolve(context.Background(), "abc")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{sourceRef: fmt.Sprintf("%s:abc@%s", dst, imageDesc.Digest)}, rewriteMapping)

	result, err := client.Pull(dst + ":2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "apiVersion: v2\nname: app\nversion: 2.0.0\n", readChartCopyTestChartFiles(t, result.Chart.Data)["app/Chart.yaml"])
}

func TestChartVersionFromTag(t *testing.T) {
	now := time.Unix(1700000000, 0)

	tests := []struct {
		tag      string
		expected string
	}{
		{tag: "1.2.3", expected: "1.2.3"},
		{tag: "v1.2.3", expected: "1.2.3"},
		{tag: "1.2.3-rc.1+build.5", expected: "1.2.3-rc.1+build.5"},
		{tag: "1.2", expected: "0.0.0-1700000000-1-2"},
		{tag: "20240101", expected: "0.0.0-1700000000-20240101"},
		{tag: "01.2.3", expected: "0.0.0-1700000000-01-2-3"},
		{tag: "latest", expected: "0.0.0-1700000000-latest"},
		{tag: "Feature/My_Branch", expected: "0.0.0-1700000000-feature-my-branch"},
		{tag: "v", expected: "0.0.0-1700000000-v"},
		{tag: "___", expected: "0.0.0-1700000000"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			version := ChartVersionFromTag(context.Background(), tt.tag, now)
			assert.Equal(t, tt.expected, version)

			_, err := semver.StrictNewVersion(version)
			assert.NoError(t, err)
		})
	}
}

func TestImageRepositoryAddress(t *testing.T) {
	tests := []struct {
		repo     string
		expected string
	}{
		{repo: "nginx", expected: "registry-1.docker.io/library/nginx"},
		{repo: "docker.io/nginx", expected: "registry-1.docker.io/library/nginx"},
		{repo: "index.docker.io/nginx", expected: "registry-1.docker.io/library/nginx"},
		{repo: "docker.io/org/app", expected: "registry-1.docker.io/org/app"},
		{repo: "org/app", expected: "registry-1.docker.io/org/app"},
		{repo: "ghcr.io/org/app", expected: "ghcr.io/org/app"},
		{repo: "localhost/app", expected: "localhost/app"},
		{repo: "127.0.0.1:5000/app", expected: "127.0.0.1:5000/app"},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			assert.Equal(t, tt.expected, imageRepositoryAddress(tt.repo))
		})
	}
}

func TestParseChartCopyAddress(t *testing.T) {
	tests := []struct {
		addr        string
		expected    chartCopyAddress
		expectedErr string
	}{
		{addr: "oci://registry.example.com/group/app:1.0.0", expected: chartCopyAddress{Repo: "registry.example.com/group/app", Tag: "1.0.0"}},
		{addr: "docker://localhost:5000/app", expected: chartCopyAddress{Repo: "localhost:5000/app"}},
		{addr: "registry.example.com/app:latest", expected: chartCopyAddress{Repo: "registry.example.com/app", Tag: "latest"}},
		{addr: "archive:path/to/bundle", expected: chartCopyAddress{ArchivePath: "path/to/bundle"}},
		{addr: "path/to/bundle.tar.gz", expected: chartCopyAddress{ArchivePath: "path/to/bundle.tar.gz"}},
		{addr: "/abs/bundle.tar", expected: chartCopyAddress{ArchivePath: "/abs/bundle.tar"}},
		{addr: "archive:", expectedErr: "archive path is empty"},
		{addr: "oci://registry.example.com/app@sha256:abc", expectedErr: "digest references are not supported"},
		{addr: "app", expectedErr: "parse repository"},
		{addr: "oci://registry.example.com/app:1.2.3+b", expectedErr: "invalid tag \"1.2.3+b\""},
		{addr: "registry.example.com/app:-bad", expectedErr: "invalid tag \"-bad\""},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			addr, err := parseChartCopyAddress(tt.addr)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, *addr)
		})
	}
}
