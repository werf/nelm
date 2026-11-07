package downloader

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/cli"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1/repotest"
)

func TestDownloadTo_VerifyIfPossibleUnsignedChart(t *testing.T) {
	srv := repotest.NewTempServer(
		t,
		repotest.WithChartSourceGlob("testdata/local-subchart-0.1.0.tgz"),
	)
	defer srv.Stop()

	newDownloader := func(out io.Writer) *ChartDownloader {
		contentCache := t.TempDir()

		return &ChartDownloader{
			Out:          out,
			Verify:       VerifyIfPossible,
			Keyring:      "testdata/helm-test-key.pub",
			ContentCache: contentCache,
			Getters:      getter.All(&cli.EnvSettings{ContentCache: contentCache}),
		}
	}

	ref := srv.URL() + "/local-subchart-0.1.0.tgz"
	expectedWarning := "WARNING: Verification not found for " + ref

	t.Run("DownloadTo", func(t *testing.T) {
		var out strings.Builder
		_, _, err := newDownloader(&out).DownloadTo(ref, "", t.TempDir())
		require.NoError(t, err)
		assert.Contains(t, out.String(), expectedWarning)
	})

	t.Run("DownloadToCache", func(t *testing.T) {
		var out strings.Builder
		_, _, err := newDownloader(&out).DownloadToCache(ref, "")
		require.NoError(t, err)
		assert.Contains(t, out.String(), expectedWarning)
	})
}

func TestDownload_ProvenanceRequest(t *testing.T) {
	tests := []struct {
		name        string
		verify      VerificationStrategy
		provStatus  int
		expectedErr string
	}{
		{
			name:   "provenance behind basic auth",
			verify: VerifyAlways,
		},
		{
			name:        "provenance server error with verify if possible",
			verify:      VerifyIfPossible,
			provStatus:  http.StatusInternalServerError,
			expectedErr: "failed to fetch provenance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileServer := http.FileServer(http.Dir("testdata"))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if username, password, ok := r.BasicAuth(); !ok || username != "username" || password != "password" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				if tt.provStatus != 0 && strings.HasSuffix(r.URL.Path, ".prov") {
					w.WriteHeader(tt.provStatus)
					return
				}

				fileServer.ServeHTTP(w, r)
			}))
			defer srv.Close()

			newDownloader := func() *ChartDownloader {
				contentCache := t.TempDir()

				return &ChartDownloader{
					Out:          io.Discard,
					Verify:       tt.verify,
					Keyring:      "testdata/helm-test-key.pub",
					ContentCache: contentCache,
					Getters:      getter.All(&cli.EnvSettings{ContentCache: contentCache}),
					Options:      []getter.Option{getter.WithBasicAuth("username", "password")},
				}
			}

			ref := srv.URL + "/signtest-0.1.0.tgz"

			t.Run("DownloadTo", func(t *testing.T) {
				_, ver, err := newDownloader().DownloadTo(ref, "", t.TempDir())
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.NotEmpty(t, ver.FileHash)
			})

			t.Run("DownloadToCache", func(t *testing.T) {
				_, ver, err := newDownloader().DownloadToCache(ref, "")
				if tt.expectedErr != "" {
					require.ErrorContains(t, err, tt.expectedErr)
					return
				}

				require.NoError(t, err)
				assert.NotEmpty(t, ver.FileHash)
			})
		})
	}
}

func TestDownloadTo_KeepsLongHTTPChartFileName(t *testing.T) {
	chartData, err := os.ReadFile("testdata/signtest-0.1.0.tgz")
	require.NoError(t, err)

	name := strings.Repeat("a", 206) + ".tgz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(chartData)
	}))
	defer srv.Close()

	downloader := ChartDownloader{
		Out:          io.Discard,
		Verify:       VerifyNever,
		Getters:      getter.All(&cli.EnvSettings{}),
		ContentCache: t.TempDir(),
	}

	dest := t.TempDir()
	path, _, err := downloader.DownloadTo(srv.URL+"/"+name, "", dest)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dest, name), path)
}

func TestBoundedFileName(t *testing.T) {
	long := strings.Repeat("я", 150) + ".tgz"
	bounded := boundedFileName(long)

	assert.LessOrEqual(t, len(bounded), maxDownloadFileNameLen)
	assert.True(t, utf8.ValidString(bounded))
	assert.True(t, strings.HasSuffix(bounded, ".tgz"))
	assert.Equal(t, "short.tgz", boundedFileName("short.tgz"))
}
