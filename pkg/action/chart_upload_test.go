package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

func TestChartUploadAndRender_LoopbackRegistryTLS(t *testing.T) {
	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	t.Setenv("HELM_DATA_HOME", t.TempDir())

	tests := []struct {
		name       string
		tls        bool
		connection common.ChartRepoConnectionOptions
	}{
		{name: "HTTPS with CA", tls: true, connection: common.ChartRepoConnectionOptions{ChartRepoCAPath: "../helm/testdata/rootca.crt"}},
		{name: "plain HTTP without TLS options"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var host string
			if tt.tls {
				host = newChartCopyTestRegistryWithTLS(t, "../helm/testdata/localhost-crt.pem", "../helm/testdata/key.pem")
			} else {
				host = newChartCopyTestRegistry(t)
			}

			archivePath := filepath.Join(t.TempDir(), "app.tgz")
			require.NoError(t, os.WriteFile(archivePath, buildChartCopyTestChart(t, map[string]string{
				"Chart.yaml":        "apiVersion: v2\nname: app\nversion: 1.0.0\n",
				"templates/cm.yaml": recordedReleaseConfigMap,
			}), 0o644))

			_, err := ChartUpload(newChartPackTestContext(), archivePath, "oci://"+host, ChartUploadOptions{
				ChartRepoConnectionOptions: tt.connection,
				RegistryCredentialsPath:    filepath.Join(t.TempDir(), "config.json"),
			})
			require.NoError(t, err)

			result, err := ChartRender(newChartRenderInstallContext(t), ChartRenderOptions{
				Chart:                      "oci://" + host + "/app:1.0.0",
				ChartRepoConnectionOptions: tt.connection,
				OutputNoPrint:              true,
				ReleaseName:                "release",
				ReleaseNamespace:           "namespace",
				RegistryCredentialsPath:    filepath.Join(t.TempDir(), "config.json"),
				TempDirPath:                t.TempDir(),
			})
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)
		})
	}
}

func TestChartUpload_ImageTagCollision(t *testing.T) {
	host := newChartCopyTestRegistry(t)

	tests := []struct {
		name        string
		image       string
		extraTags   []string
		expectedErr string
	}{
		{
			name:        "image tag is the version tag",
			image:       host + "/collision1/app:1.0.0_b",
			expectedErr: fmt.Sprintf(`chart tag "1.0.0_b" would overwrite the image "backend" %s/collision1/app:1.0.0_b in the same repository`, host),
		},
		{
			name:        "image tag is an extra tag",
			image:       host + "/collision2/app:latest@sha256:" + fmt.Sprintf("%064d", 0),
			extraTags:   []string{"latest"},
			expectedErr: `chart tag "latest" would overwrite the image "backend"`,
		},
		{
			name:  "image tag in another repository",
			image: host + "/images:1.0.0_b",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := writeChartUploadTestArchive(t, "apiVersion: v2\nname: app\nversion: 1.0.0+b\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: "+tt.image+"\n")
			remote := fmt.Sprintf("oci://%s/collision%d", host, i+1)
			repo := fmt.Sprintf("%s/collision%d/app", host, i+1)

			opts := newChartUploadTestOptions(t)
			opts.ExtraTags = tt.extraTags

			_, err := ChartUpload(newChartPackTestContext(), archivePath, remote, opts)
			if tt.expectedErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, tt.expectedErr)

			var pushedTags []string

			_ = newChartCopyTestRepository(t, repo).Tags(context.Background(), "", func(tags []string) error {
				pushedTags = append(pushedTags, tags...)

				return nil
			})
			assert.Empty(t, pushedTags)
		})
	}
}

func TestChartUpload_InvalidImagesAnnotation(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	archivePath := writeChartUploadTestArchive(t, "apiVersion: v2\nname: app\nversion: 1.0.0\nannotations:\n  helm.sh/images: not a list\n")

	_, err := ChartUpload(newChartPackTestContext(), archivePath, "oci://"+host, newChartUploadTestOptions(t))
	require.NoError(t, err)
}

func TestChartUpload_LoopbackPlainHTTP(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	archivePath := writeChartUploadTestArchive(t, "apiVersion: v2\nname: app\nversion: 1.0.0\n")

	opts := newChartUploadTestOptions(t)
	opts.ChartRepoInsecure = false

	_, err := ChartUpload(newChartPackTestContext(), archivePath, "oci://"+host, opts)
	require.NoError(t, err)

	_, err = newChartCopyTestRegistryClient(t).Resolve(host + "/app:1.0.0")
	require.NoError(t, err)
}

func TestChartUpload_Remote(t *testing.T) {
	archivePath := writeChartUploadTestArchive(t, "apiVersion: v2\nname: app\nversion: 1.0.0\n")

	tests := []struct {
		name        string
		remote      string
		expectedErr string
	}{
		{name: "without OCI scheme", remote: "localhost:5000/charts", expectedErr: `remote "localhost:5000/charts" must start with "oci://"`},
		{name: "with tag", remote: "oci://localhost:5000/charts:1.0.0", expectedErr: `parse repository "localhost:5000/charts:1.0.0/app"`},
		{name: "with digest", remote: "oci://localhost:5000/charts@sha256:" + fmt.Sprintf("%064d", 0), expectedErr: "parse repository"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ChartUpload(newChartPackTestContext(), archivePath, tt.remote, newChartUploadTestOptions(t))
			require.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestChartUpload_RepositoryOfChartName(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	archivePath := writeChartUploadTestArchive(t, "apiVersion: v2\nname: app\nversion: 1.0.0\n")

	tests := []struct {
		name         string
		remote       string
		expectedRepo string
	}{
		{name: "host", remote: "oci://" + host, expectedRepo: host + "/app"},
		{name: "host and path", remote: "oci://" + host + "/charts/team", expectedRepo: host + "/charts/team/app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ChartUpload(newChartPackTestContext(), archivePath, tt.remote, newChartUploadTestOptions(t))
			require.NoError(t, err)

			_, err = newChartCopyTestRegistryClient(t).Resolve(tt.expectedRepo + ":1.0.0")
			require.NoError(t, err)
		})
	}
}

func TestChartUpload_Tags(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	tests := []struct {
		name         string
		version      string
		extraTags    []string
		provenance   bool
		expectedTags []string
	}{
		{
			name:         "version tag only",
			version:      "1.0.0",
			expectedTags: []string{"1.0.0"},
		},
		{
			name:         "extra tags with plus encoded and deduplicated",
			version:      "1.0.0+b",
			extraTags:    []string{"latest", "1.0.0+b", "v1+x", "latest", "v1_x"},
			expectedTags: []string{"1.0.0_b", "latest", "v1_x"},
		},
		{
			name:         "provenance",
			version:      "1.0.0",
			extraTags:    []string{"stable"},
			provenance:   true,
			expectedTags: []string{"1.0.0", "stable"},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartData := buildChartCopyTestChart(t, map[string]string{"Chart.yaml": "apiVersion: v2\nname: app\nversion: " + tt.version + "\n"})
			archivePath := filepath.Join(t.TempDir(), "app.tgz")
			require.NoError(t, os.WriteFile(archivePath, chartData, 0o644))

			var provData []byte
			if tt.provenance {
				provData = signChartCopyTestChart(t, chartData)
				require.NoError(t, os.WriteFile(archivePath+".prov", provData, 0o644))
			}

			repo := fmt.Sprintf("%s/tags%d/app", host, i)

			opts := newChartUploadTestOptions(t)
			opts.ExtraTags = tt.extraTags

			result, err := ChartUpload(newChartPackTestContext(), archivePath, fmt.Sprintf("oci://%s/tags%d", host, i), opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedTags, result.Tags)

			var pushedTags []string

			require.NoError(t, newChartCopyTestRepository(t, repo).Tags(context.Background(), "", func(tags []string) error {
				pushedTags = append(pushedTags, tags...)

				return nil
			}))
			assert.ElementsMatch(t, tt.expectedTags, pushedTags)

			for _, tag := range tt.expectedTags {
				pullResult, err := client.Pull(repo+":"+tag, registry.PullOptWithProv(true), registry.PullOptIgnoreMissingProv(true))
				require.NoError(t, err, tag)
				assert.Equal(t, result.ManifestDigest, pullResult.Manifest.Digest, tag)
				assert.Equal(t, tt.version, pullResult.Chart.Meta.Version, tag)

				if tt.provenance {
					assert.Equal(t, provData, pullResult.Prov.Data, tag)
				} else {
					assert.Empty(t, pullResult.Prov.Data, tag)
				}
			}
		})
	}
}

func newChartUploadTestOptions(t *testing.T) ChartUploadOptions {
	t.Helper()

	return ChartUploadOptions{
		ChartRepoConnectionOptions: common.ChartRepoConnectionOptions{ChartRepoInsecure: true},
		RegistryCredentialsPath:    filepath.Join(t.TempDir(), "config.json"),
	}
}

func writeChartUploadTestArchive(t *testing.T, chartYAML string) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "app.tgz")
	require.NoError(t, os.WriteFile(archivePath, buildChartCopyTestChart(t, map[string]string{"Chart.yaml": chartYAML}), 0o644))

	return archivePath
}
