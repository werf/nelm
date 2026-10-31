package action

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content"

	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

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
		nginxRef:     dst + ":1.25",
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
			name:            "semver tag with build metadata",
			to:              host + "/build/app:1.2.3+b",
			expectedVersion: "^1\\.2\\.3\\+b$",
			expectedTags:    []string{"1.2.3_b"},
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
		"plain/layer.tar":   []byte("plain layer"),
	})

	sourceRef := "old.registry/project:abc@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	chartData := buildChartCopyTestChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: project\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: " + sourceRef + "\n",
	})

	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, gzipChartCopyTestData(t, tarChartCopyTestFiles(t, map[string][]byte{
		werfArchiveChartFileName: chartData,
		"images/abc.tar.gz":      gzipChartCopyTestData(t, imageTarball),
	})), 0o644))

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
	assert.Equal(t, "plain layer", string(plainLayerData))

	result, err := client.Pull(dst + ":2.0.0")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("apiVersion: v2\nname: app\nversion: 2.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: %s:abc@%s\n", dst, manifestDesc.Digest), readChartCopyTestChartFiles(t, result.Chart.Data)["app/Chart.yaml"])
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
