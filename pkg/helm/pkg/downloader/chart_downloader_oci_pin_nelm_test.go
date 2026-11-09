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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/level"
	"github.com/werf/nelm/v2/pkg/helm/pkg/cli"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/provenance"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1/repotest"
)

const (
	pinTestKeyfile = "testdata/helm-test-key.secret"
	pinTestKeyring = "testdata/helm-test-key.pub"
	pinTestSigner  = "Helm Testing (This key should only be used for testing. DO NOT TRUST.) <helm-testing@helm.sh>"
)

func TestChartDownloader_OCITagBindsChartVersion(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	for _, chart := range []struct{ tag, version string }{
		{tag: "1.2.3", version: "1.2.3"},
		{tag: "v1.2.3", version: "1.2.3"},
		{tag: "1.2.3_1", version: "1.2.3+1"},
		{tag: "latest", version: "1.2.3"},
		{tag: "1.0.0_rc1", version: "0.0.0-1700000000-1-0-0-rc1"},
		{tag: "2.0.0", version: "1.2.3"},
		{tag: "vv1.2.3", version: "1.2.3"},
		{tag: "v2.0.0", version: "1.2.3"},
		{tag: "1.2.3_2", version: "1.2.3+1"},
		{tag: "v1.2.4", version: "v1.2.4"},
		{tag: "1.2.5", version: "v1.2.5"},
		{tag: "v1.2.6", version: "v1.2.7"},
	} {
		pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:"+chart.tag, chart.version, true)
	}

	tests := []struct {
		name        string
		tag         string
		verify      VerificationStrategy
		expectedErr string
	}{
		{name: "version tag", tag: "1.2.3", verify: VerifyAlways},
		{name: "version tag with v", tag: "v1.2.3", verify: VerifyAlways},
		{name: "version tag with build metadata", tag: "1.2.3_1", verify: VerifyAlways},
		{name: "non-version tag", tag: "latest", verify: VerifyAlways},
		{name: "legacy bundle tag not verified", tag: "1.0.0_rc1", verify: VerifyNever},
		{name: "legacy bundle tag verified", tag: "1.0.0_rc1", verify: VerifyAlways, expectedErr: `chart version "0.0.0-1700000000-1-0-0-rc1" does not match the requested tag "1.0.0_rc1"`},
		{name: "version tag of another version verified if possible", tag: "2.0.0", verify: VerifyIfPossible, expectedErr: `chart version "1.2.3" does not match the requested tag "2.0.0"`},
		{name: "version tag of another version not verified", tag: "2.0.0", verify: VerifyNever},
		{name: "version tag of another version verified later", tag: "2.0.0", verify: VerifyLater},
		{name: "tag with two v is no version", tag: "vv1.2.3", verify: VerifyAlways},
		{name: "version tag with v of another version", tag: "v2.0.0", verify: VerifyAlways, expectedErr: `chart version "1.2.3" does not match the requested tag "v2.0.0"`},
		{name: "version tag with other build metadata", tag: "1.2.3_2", verify: VerifyAlways, expectedErr: `chart version "1.2.3+1" does not match the requested tag "1.2.3_2"`},
		{name: "version with v and tag with v", tag: "v1.2.4", verify: VerifyAlways},
		{name: "version with v and tag without v", tag: "1.2.5", verify: VerifyAlways},
		{name: "version with v of another version", tag: "v1.2.6", verify: VerifyAlways, expectedErr: `chart version "v1.2.7" does not match the requested tag "v1.2.6"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := "oci://" + srv.RegistryURL + "/u/test/app:" + tt.tag

			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					_, _, err := download.run(t, newPinTestDownloader(t, registryClient, tt.verify, pinTestKeyring), ref, "")
					if tt.expectedErr != "" {
						require.ErrorContains(t, err, tt.expectedErr)
						return
					}

					require.NoError(t, err)
				})
			}
		})
	}
}

func TestChartDownloader_OCIChartAndProvenanceFromOneManifest(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.2.3", "1.2.3", true)
	movedResult := pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:moved", "1.2.4", true)

	target, err := url.Parse("http://" + srv.RegistryURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)

	var (
		tagResolved atomic.Bool
		tagRequests atomic.Int32
	)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetBasicAuth(srv.TestUsername, srv.TestPassword)

		// The tag moves to another chart once it has been resolved.
		if strings.HasSuffix(r.URL.Path, "/manifests/1.2.3") {
			tagRequests.Add(1)
			if tagResolved.Swap(true) {
				r.URL.Path = strings.TrimSuffix(r.URL.Path, "1.2.3") + movedResult.Manifest.Digest
			}
		}

		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(proxySrv.Close)

	proxyClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	for _, download := range pinTestDownloads() {
		t.Run(download.name, func(t *testing.T) {
			tagResolved.Store(false)
			tagRequests.Store(0)

			_, ver, err := download.run(t, newPinTestDownloader(t, proxyClient, VerifyAlways, pinTestKeyring), "oci://"+strings.TrimPrefix(proxySrv.URL, "http://")+"/u/test/app:1.2.3", "")
			require.NoError(t, err)
			assert.Equal(t, "app-1.2.3.tgz", ver.FileName)
			assert.Equal(t, int32(1), tagRequests.Load())
		})
	}
}

func TestChartDownloader_OCIProvenanceFromManifestNotCache(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.2.3", "1.2.3", true)
	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/unsigned/app:1.2.3", "1.2.3", false)
	pushPinTestChartSignedAs(t, registryClient, srv.RegistryURL+"/u/resigned/app:1.2.3", "1.2.3", "other-1.2.3.tgz")

	tests := []struct {
		name        string
		ref         string
		expectedErr string
	}{
		{name: "identical unsigned archive", ref: "oci://" + srv.RegistryURL + "/u/unsigned/app:1.2.3", expectedErr: "failed to fetch provenance"},
		{name: "identical archive signed differently", ref: "oci://" + srv.RegistryURL + "/u/resigned/app:1.2.3", expectedErr: "app-1.2.3.tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					contentCache := t.TempDir()

					warm := newPinTestDownloader(t, registryClient, VerifyAlways, pinTestKeyring)
					warm.ContentCache = contentCache
					_, _, err := download.run(t, warm, "oci://"+srv.RegistryURL+"/u/test/app:1.2.3", "")
					require.NoError(t, err)

					dl := newPinTestDownloader(t, registryClient, VerifyAlways, pinTestKeyring)
					dl.ContentCache = contentCache
					_, _, err = download.run(t, dl, tt.ref, "")
					require.ErrorContains(t, err, tt.expectedErr)
				})
			}
		})
	}
}

func TestChartDownloader_OCIChartVersionOption(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.2.3", "1.2.3", true)

	for _, download := range pinTestDownloads() {
		t.Run(download.name, func(t *testing.T) {
			var out bytes.Buffer
			ctx := newPinTestLogContext(&out)

			dl := newPinTestDownloader(t, registryClient, VerifyAlways, pinTestKeyring)
			dl.Options = append(dl.Options, getter.WithContext(ctx))

			_, ver, err := download.run(t, dl, "oci://"+srv.RegistryURL+"/u/test/app", "1.2.3")
			require.NoError(t, err)
			assert.Equal(t, "app-1.2.3.tgz", ver.FileName)
			assert.Contains(t, out.String(), "Resolved chart oci://"+srv.RegistryURL+"/u/test/app:1.2.3 to digest sha256:")
			assert.Contains(t, out.String(), "Verified chart oci://"+srv.RegistryURL+"/u/test/app:1.2.3 signed by "+pinTestSigner)
		})
	}
}

func TestChartDownloader_OCIProvTagRejectedBeforeRequests(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	host := strings.TrimPrefix(srv.URL, "http://")

	tests := []struct {
		name    string
		ref     string
		version string
	}{
		{name: "tag in reference", ref: "oci://" + host + "/u/test/app:1.2.3.prov"},
		{name: "tag as version", ref: "oci://" + host + "/u/test/app", version: "1.2.3.prov"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					_, _, err := download.run(t, newPinTestDownloader(t, registryClient, VerifyNever, ""), tt.ref, tt.version)
					require.ErrorContains(t, err, `tag "1.2.3.prov" is not allowed: tags ending in ".prov" address provenance files`)
					assert.Zero(t, requests.Load())
				})
			}
		})
	}
}

func TestChartDownloader_OCIKeyringRequired(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.2.3", "1.2.3", true)
	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/unsigned:1.2.3", "1.2.3", false)

	var requests atomic.Int32
	countingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(countingSrv.Close)

	countingClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	tests := []struct {
		name           string
		ref            string
		registryClient *registry.Client
		verify         VerificationStrategy
		expectedErr    error
	}{
		{name: "verify without keyring", ref: "oci://" + strings.TrimPrefix(countingSrv.URL, "http://") + "/u/test/app:1.2.3", registryClient: countingClient, verify: VerifyAlways, expectedErr: ErrKeyringRequired},
		{name: "verify if possible signed chart without keyring", ref: "oci://" + srv.RegistryURL + "/u/test/app:1.2.3", registryClient: registryClient, verify: VerifyIfPossible, expectedErr: ErrKeyringRequired},
		{name: "verify if possible unsigned chart without keyring", ref: "oci://" + srv.RegistryURL + "/u/test/unsigned:1.2.3", registryClient: registryClient, verify: VerifyIfPossible},
		{name: "verify later without keyring", ref: "oci://" + srv.RegistryURL + "/u/test/app:1.2.3", registryClient: registryClient, verify: VerifyLater},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					_, _, err := download.run(t, newPinTestDownloader(t, tt.registryClient, tt.verify, ""), tt.ref, "")
					if tt.expectedErr != nil {
						require.ErrorIs(t, err, tt.expectedErr)
						return
					}

					require.NoError(t, err)
				})
			}
		})
	}

	assert.Zero(t, requests.Load())
}

func TestChartDownloader_OCICanceled(t *testing.T) {
	requested := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		select {
		case requested <- struct{}{}:
		default:
		}
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dl := newPinTestDownloader(t, registryClient, VerifyNever, "")
	dl.Options = append(dl.Options, getter.WithContext(ctx))

	errCh := make(chan error, 1)
	go func() {
		_, _, err := dl.DownloadTo("oci://"+strings.TrimPrefix(srv.URL, "http://")+"/u/test/app:1.2.3", "", t.TempDir())
		errCh <- err
	}()

	select {
	case <-requested:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "request did not reach the server")
	}

	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "download did not return after the context was cancelled")
	}
}

func TestChartDownloader_OCIChartVersionCanceled(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, version := range []string{"latest", "^1.0.0"} {
		t.Run(version, func(t *testing.T) {
			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					dl := newPinTestDownloader(t, registryClient, VerifyNever, "")
					dl.Options = append(dl.Options, getter.WithContext(ctx))

					_, _, err := download.run(t, dl, "oci://"+strings.TrimPrefix(srv.URL, "http://")+"/u/test/app", version)
					require.ErrorIs(t, err, context.Canceled)
				})
			}
		})
	}

	assert.Zero(t, requests.Load())
}

func TestChartDownloader_OCIChartVersionTagResolvedOnce(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:latest", "1.2.3", true)
	movedResult := pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:moved", "1.2.4", true)

	target, err := url.Parse("http://" + srv.RegistryURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)

	var (
		tagResolved atomic.Bool
		tagRequests atomic.Int32
	)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetBasicAuth(srv.TestUsername, srv.TestPassword)

		// The tag moves to another chart once it has been resolved.
		if strings.HasSuffix(r.URL.Path, "/manifests/latest") {
			tagRequests.Add(1)
			if tagResolved.Swap(true) {
				r.URL.Path = strings.TrimSuffix(r.URL.Path, "latest") + movedResult.Manifest.Digest
			}
		}

		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(proxySrv.Close)

	proxyClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	for _, download := range pinTestDownloads() {
		t.Run(download.name, func(t *testing.T) {
			tagResolved.Store(false)
			tagRequests.Store(0)

			_, ver, err := download.run(t, newPinTestDownloader(t, proxyClient, VerifyAlways, pinTestKeyring), "oci://"+strings.TrimPrefix(proxySrv.URL, "http://")+"/u/test/app", "latest")
			require.NoError(t, err)
			assert.Equal(t, "app-1.2.3.tgz", ver.FileName)
			assert.Equal(t, int32(1), tagRequests.Load())
		})
	}
}

func TestChartDownloader_OCIConcurrentDownloadsToSharedCache(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.2.3", "1.2.3", true)

	contentCache := t.TempDir()
	dest := t.TempDir()

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 64 {
		wg.Go(func() {
			dl := newPinTestDownloader(t, registryClient, VerifyAlways, pinTestKeyring)
			dl.ContentCache = contentCache

			var err error
			if i%4 == 0 {
				_, _, err = dl.DownloadTo("oci://"+srv.RegistryURL+"/u/test/app:1.2.3", "", dest)
			} else {
				_, _, err = dl.DownloadToCache("oci://"+srv.RegistryURL+"/u/test/app:1.2.3", "")
			}

			errs <- err
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}

type pinTestDownload struct {
	name string
	run  func(t *testing.T, dl *ChartDownloader, ref, version string) (string, *provenance.Verification, error)
}

func pinTestDownloads() []pinTestDownload {
	return []pinTestDownload{
		{
			name: "DownloadTo",
			run: func(t *testing.T, dl *ChartDownloader, ref, version string) (string, *provenance.Verification, error) {
				return dl.DownloadTo(ref, version, t.TempDir())
			},
		},
		{
			name: "DownloadToCache",
			run: func(_ *testing.T, dl *ChartDownloader, ref, version string) (string, *provenance.Verification, error) {
				return dl.DownloadToCache(ref, version)
			},
		},
	}
}

func newPinTestRegistry(t *testing.T) (*repotest.OCIServer, *registry.Client) {
	t.Helper()

	srv, err := repotest.NewOCIServer(t, t.TempDir())
	require.NoError(t, err)

	go srv.ListenAndServe()

	registryClient, err := registry.NewClient(
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(filepath.Join(srv.Dir, "config.json")),
	)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return registryClient.Login(
			srv.RegistryURL,
			registry.LoginOptBasicAuth(srv.TestUsername, srv.TestPassword),
			registry.LoginOptInsecure(true),
			registry.LoginOptPlainText(true),
		) == nil
	}, 10*time.Second, 50*time.Millisecond)

	return srv, registryClient
}

func newPinTestDownloader(t *testing.T, registryClient *registry.Client, verify VerificationStrategy, keyring string) *ChartDownloader {
	t.Helper()

	contentCache := t.TempDir()

	return &ChartDownloader{
		Out:            io.Discard,
		Verify:         verify,
		Keyring:        keyring,
		Getters:        getter.All(&cli.EnvSettings{ContentCache: contentCache}),
		Options:        []getter.Option{getter.WithRegistryClient(registryClient)},
		RegistryClient: registryClient,
		ContentCache:   contentCache,
	}
}

func newPinTestLogContext(out io.Writer) context.Context {
	logger := logboek.NewLogger(out, out)
	logger.SetAcceptedLevel(level.Default)
	logger.Streams().SetWidth(1000)

	return logboek.NewContext(context.Background(), logger)
}

// pushPinTestChart pushes a chart app of version under ref, signed if sign, bypassing the strict
// mode so that the tag can differ from the version.
func pushPinTestChart(t *testing.T, registryClient *registry.Client, ref, version string, sign bool) *registry.PushResult {
	t.Helper()

	signedName := ""
	if sign {
		signedName = "app-" + version + ".tgz"
	}

	return pushPinTestChartSignedAs(t, registryClient, ref, version, signedName)
}

// pushPinTestChartSignedAs pushes a chart app of version under ref with a provenance signing it
// under signedName, unsigned if signedName is empty.
func pushPinTestChartSignedAs(t *testing.T, registryClient *registry.Client, ref, version, signedName string) *registry.PushResult {
	t.Helper()

	chartYAML, err := yaml.Marshal(map[string]string{"apiVersion": "v2", "name": "app", "version": version})
	require.NoError(t, err)

	var chartData bytes.Buffer
	gzipWriter := gzip.NewWriter(&chartData)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "app/Chart.yaml", Mode: 0o644, Size: int64(len(chartYAML)), Typeflag: tar.TypeReg}))
	_, err = tarWriter.Write(chartYAML)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	opts := []registry.PushOption{registry.PushOptStrictMode(false)}
	if signedName != "" {
		signer, err := provenance.NewFromFiles(pinTestKeyfile, pinTestKeyring)
		require.NoError(t, err)

		prov, err := signer.ClearSign(chartData.Bytes(), signedName, chartYAML)
		require.NoError(t, err)

		opts = append(opts, registry.PushOptProvData([]byte(prov)))
	}

	result, err := registryClient.Push(chartData.Bytes(), ref, opts...)
	require.NoError(t, err)

	return result
}

func TestChartDownloader_OCILiteralTag(t *testing.T) {
	srv, registryClient := newPinTestRegistry(t)

	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/test/app:1.0.0", "1.0.0", false)
	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/present/app:1.0.0", "1.0.0", false)
	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/present/app:1.x", "1.1.0", false)
	pushPinTestChart(t, registryClient, srv.RegistryURL+"/u/present/app:1.2.3_1", "1.2.3+1", false)

	target, err := url.Parse("http://" + srv.RegistryURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)

	var tagListRequests atomic.Int32
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetBasicAuth(srv.TestUsername, srv.TestPassword)

		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			tagListRequests.Add(1)
		}

		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(proxySrv.Close)

	proxyClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	host := strings.TrimPrefix(proxySrv.URL, "http://")

	tests := []struct {
		name            string
		ref             string
		version         string
		literal         bool
		expectedVersion string
		expectedErr     string
		expectTagList   bool
	}{
		{name: "missing tag in reference", ref: "oci://" + host + "/u/test/app:1.x", literal: true, expectedErr: "not found"},
		{name: "missing tag as version", ref: "oci://" + host + "/u/test/app", version: "1.x", literal: true, expectedErr: "not found"},
		{name: "present tag in reference", ref: "oci://" + host + "/u/present/app:1.x", literal: true, expectedVersion: "1.1.0"},
		{name: "present tag as version", ref: "oci://" + host + "/u/present/app", version: "1.x", literal: true, expectedVersion: "1.1.0"},
		{name: "encoded tag", ref: "oci://" + host + "/u/present/app:1.2.3+1", literal: true, expectedVersion: "1.2.3+1"},
		{name: "no tag", ref: "oci://" + host + "/u/present/app", literal: true, expectedErr: "has no tag"},
		{name: "reference and version mismatch", ref: "oci://" + host + "/u/present/app:1.x", version: "1.0.0", literal: true, expectedErr: "chart reference and version mismatch"},
		{name: "constraint matched by default", ref: "oci://" + host + "/u/test/app:1.x", expectedVersion: "1.0.0", expectTagList: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, download := range pinTestDownloads() {
				t.Run(download.name, func(t *testing.T) {
					tagListRequests.Store(0)

					dl := newPinTestDownloader(t, proxyClient, VerifyNever, "")
					dl.LiteralTag = tt.literal

					chartPath, _, err := download.run(t, dl, tt.ref, tt.version)
					if tt.expectTagList {
						assert.NotZero(t, tagListRequests.Load())
					} else {
						assert.Zero(t, tagListRequests.Load())
					}

					if tt.expectedErr != "" {
						require.ErrorContains(t, err, tt.expectedErr)
						return
					}

					require.NoError(t, err)

					archiveData, err := os.ReadFile(chartPath)
					require.NoError(t, err)

					_, version, err := chartArchiveNameVersion(archiveData)
					require.NoError(t, err)
					assert.Equal(t, tt.expectedVersion, version)
				})
			}
		})
	}
}
