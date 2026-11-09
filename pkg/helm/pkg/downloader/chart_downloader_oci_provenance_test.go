package downloader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/werf/nelm/v2/pkg/helm/pkg/cli"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1/repotest"
)

func TestChartDownloader_VerifiesOCIChartPulledByNonVersionTag(t *testing.T) {
	srv, err := repotest.NewOCIServer(t, t.TempDir())
	require.NoError(t, err)

	go srv.ListenAndServe()

	registryClient, err := registry.NewClient(
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(filepath.Join(srv.Dir, "config.json")),
	)
	require.NoError(t, err)
	require.NoError(t, registryClient.Login(
		srv.RegistryURL,
		registry.LoginOptBasicAuth(srv.TestUsername, srv.TestPassword),
		registry.LoginOptInsecure(true),
		registry.LoginOptPlainText(true),
	))

	chartData, err := os.ReadFile("testdata/signtest-0.1.0.tgz")
	require.NoError(t, err)
	provData, err := os.ReadFile("testdata/signtest-0.1.0.tgz.prov")
	require.NoError(t, err)

	signedResult, err := registryClient.Push(chartData, srv.RegistryURL+"/u/test/signtest:0.1.0", registry.PushOptProvData(provData), registry.PushOptExtraTags([]string{"latest"}))
	require.NoError(t, err)

	_, err = registryClient.Push(chartData, srv.RegistryURL+"/u/test/signtest:0.2.0", registry.PushOptProvData(provData), registry.PushOptStrictMode(false))
	require.NoError(t, err)

	longRepository := "u/test/" + strings.Repeat("a", 200)
	longRepositoryResult, err := registryClient.Push(chartData, srv.RegistryURL+"/"+longRepository+":0.1.0", registry.PushOptProvData(provData), registry.PushOptStrictMode(false))
	require.NoError(t, err)

	tamperedProvData := bytes.Replace(provData, []byte("name: signtest"), []byte("name: tampered"), 1)
	require.NotEqual(t, provData, tamperedProvData)
	_, err = registryClient.Push(chartData, srv.RegistryURL+"/u/test/tampered:latest", registry.PushOptProvData(tamperedProvData), registry.PushOptStrictMode(false))
	require.NoError(t, err)

	tests := []struct {
		name        string
		ref         string
		expectedErr string
	}{
		{name: "version tag", ref: "oci://" + srv.RegistryURL + "/u/test/signtest:0.1.0"},
		{name: "non-version tag", ref: "oci://" + srv.RegistryURL + "/u/test/signtest:latest"},
		{name: "digest", ref: "oci://" + srv.RegistryURL + "/u/test/signtest@" + signedResult.Manifest.Digest},
		{name: "digest of a repository with a long basename", ref: "oci://" + srv.RegistryURL + "/" + longRepository + "@" + longRepositoryResult.Manifest.Digest},
		{name: "chart of another version under a version tag", ref: "oci://" + srv.RegistryURL + "/u/test/signtest:0.2.0", expectedErr: `chart version "0.1.0" does not match the requested tag "0.2.0"`},
		{name: "tampered provenance", ref: "oci://" + srv.RegistryURL + "/u/test/tampered:latest", expectedErr: "openpgp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDownloader := func() *ChartDownloader {
				contentCache := t.TempDir()

				return &ChartDownloader{
					Out:            io.Discard,
					Verify:         VerifyAlways,
					Keyring:        "testdata/helm-test-key.pub",
					Getters:        getter.All(&cli.EnvSettings{ContentCache: contentCache}),
					Options:        []getter.Option{getter.WithRegistryClient(registryClient)},
					RegistryClient: registryClient,
					ContentCache:   contentCache,
				}
			}

			t.Run("DownloadTo", func(t *testing.T) {
				_, ver, err := newDownloader().DownloadTo(tt.ref, "", t.TempDir())
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, "signtest-0.1.0.tgz", ver.FileName)
			})

			t.Run("DownloadToCache", func(t *testing.T) {
				_, ver, err := newDownloader().DownloadToCache(tt.ref, "")
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, "signtest-0.1.0.tgz", ver.FileName)
			})
		})
	}
}

func TestChartDownloader_VerifyIfPossibleOCIChart(t *testing.T) {
	srv, err := repotest.NewOCIServer(t, t.TempDir())
	require.NoError(t, err)

	go srv.ListenAndServe()

	registryClient, err := registry.NewClient(
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(filepath.Join(srv.Dir, "config.json")),
	)
	require.NoError(t, err)
	require.NoError(t, registryClient.Login(
		srv.RegistryURL,
		registry.LoginOptBasicAuth(srv.TestUsername, srv.TestPassword),
		registry.LoginOptInsecure(true),
		registry.LoginOptPlainText(true),
	))

	chartData, err := os.ReadFile("testdata/signtest-0.1.0.tgz")
	require.NoError(t, err)
	provData, err := os.ReadFile("testdata/signtest-0.1.0.tgz.prov")
	require.NoError(t, err)

	signedResult, err := registryClient.Push(chartData, srv.RegistryURL+"/u/test/signtest:0.1.0", registry.PushOptProvData(provData))
	require.NoError(t, err)

	_, err = registryClient.Push(chartData, srv.RegistryURL+"/u/test/unsigned:latest", registry.PushOptStrictMode(false))
	require.NoError(t, err)

	target, err := url.Parse("http://" + srv.RegistryURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	failingProvServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/blobs/"+signedResult.Prov.Digest) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.SetBasicAuth(srv.TestUsername, srv.TestPassword)
		proxy.ServeHTTP(w, r)
	}))
	defer failingProvServer.Close()

	failingProvRegistryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	tests := []struct {
		name            string
		ref             string
		registryClient  *registry.Client
		expectedErr     string
		expectedWarning string
	}{
		{
			name:            "unsigned chart",
			ref:             "oci://" + srv.RegistryURL + "/u/test/unsigned:latest",
			registryClient:  registryClient,
			expectedWarning: "WARNING: Verification not found for oci://" + srv.RegistryURL + "/u/test/unsigned:latest",
		},
		{
			name:           "signed chart whose provenance cannot be fetched",
			ref:            "oci://" + strings.TrimPrefix(failingProvServer.URL, "http://") + "/u/test/signtest:0.1.0",
			registryClient: failingProvRegistryClient,
			expectedErr:    "failed to fetch provenance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDownloader := func(out io.Writer) *ChartDownloader {
				contentCache := t.TempDir()

				return &ChartDownloader{
					Out:            out,
					Verify:         VerifyIfPossible,
					Keyring:        "testdata/helm-test-key.pub",
					Getters:        getter.All(&cli.EnvSettings{ContentCache: contentCache}),
					Options:        []getter.Option{getter.WithRegistryClient(tt.registryClient)},
					RegistryClient: tt.registryClient,
					ContentCache:   contentCache,
				}
			}

			t.Run("DownloadTo", func(t *testing.T) {
				var out strings.Builder
				_, _, err := newDownloader(&out).DownloadTo(tt.ref, "", t.TempDir())
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.Contains(t, out.String(), tt.expectedWarning)
			})

			t.Run("DownloadToCache", func(t *testing.T) {
				var out strings.Builder
				_, _, err := newDownloader(&out).DownloadToCache(tt.ref, "")
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.Contains(t, out.String(), tt.expectedWarning)
			})
		})
	}
}

func TestChartDownloader_OCIChartNameIsNotAPath(t *testing.T) {
	srv, err := repotest.NewOCIServer(t, t.TempDir())
	require.NoError(t, err)

	go srv.ListenAndServe()

	registryClient, err := registry.NewClient(
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(filepath.Join(srv.Dir, "config.json")),
	)
	require.NoError(t, err)
	require.NoError(t, registryClient.Login(
		srv.RegistryURL,
		registry.LoginOptBasicAuth(srv.TestUsername, srv.TestPassword),
		registry.LoginOptInsecure(true),
		registry.LoginOptPlainText(true),
	))

	var chartData bytes.Buffer
	gzipWriter := gzip.NewWriter(&chartData)
	tarWriter := tar.NewWriter(gzipWriter)
	chartYAML := []byte("apiVersion: v2\nname: ../victim\nversion: 1.0.0\n")
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "victim/Chart.yaml", Mode: 0o644, Size: int64(len(chartYAML))}))
	_, err = tarWriter.Write(chartYAML)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	provData, err := os.ReadFile("testdata/signtest-0.1.0.tgz.prov")
	require.NoError(t, err)

	pushUnvalidatedChart(t, srv, "u/test/victim", "latest", chartData.Bytes(), provData)

	contentCache := filepath.Join(t.TempDir(), "cache")
	sentinel := filepath.Join(filepath.Dir(contentCache), "victim-1.0.0.tgz")
	require.NoError(t, os.WriteFile(sentinel, []byte("sentinel"), 0o644))

	downloader := &ChartDownloader{
		Out:            io.Discard,
		Verify:         VerifyAlways,
		Keyring:        "testdata/helm-test-key.pub",
		Getters:        getter.All(&cli.EnvSettings{ContentCache: contentCache}),
		Options:        []getter.Option{getter.WithRegistryClient(registryClient)},
		RegistryClient: registryClient,
		ContentCache:   contentCache,
	}

	_, _, err = downloader.DownloadToCache("oci://"+srv.RegistryURL+"/u/test/victim:latest", "")
	require.Error(t, err)

	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "sentinel", string(data))
}

// pushUnvalidatedChart pushes a chart manifest bypassing the client's chart validation, as a
// hostile registry could serve it.
func pushUnvalidatedChart(t *testing.T, srv *repotest.OCIServer, repository, tag string, chartData, provData []byte) {
	t.Helper()

	ctx := context.Background()
	store := memory.New()

	pushBlob := func(mediaType string, data []byte) ocispec.Descriptor {
		desc := content.NewDescriptorFromBytes(mediaType, data)
		require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))

		return desc
	}

	configDesc := pushBlob(registry.ConfigMediaType, []byte(`{"name":"victim","version":"1.0.0"}`))
	layers := []ocispec.Descriptor{
		pushBlob(registry.ChartLayerMediaType, chartData),
		pushBlob(registry.ProvLayerMediaType, provData),
	}

	manifestDesc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "", oras.PackManifestOptions{
		ConfigDescriptor: &configDesc,
		Layers:           layers,
	})
	require.NoError(t, err)
	require.NoError(t, store.Tag(ctx, manifestDesc, tag))

	repo, err := remote.NewRepository(srv.RegistryURL + "/" + repository)
	require.NoError(t, err)
	repo.PlainHTTP = true
	repo.Client = &auth.Client{
		Credential: auth.StaticCredential(srv.RegistryURL, auth.Credential{Username: srv.TestUsername, Password: srv.TestPassword}),
	}

	_, err = oras.Copy(ctx, store, tag, repo, tag, oras.DefaultCopyOptions)
	require.NoError(t, err)
}
