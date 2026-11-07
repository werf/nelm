package chart

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

func TestDownloadChart_Canceled(t *testing.T) {
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

	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())

	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := downloadChart(ctx, "oci://"+strings.TrimPrefix(srv.URL, "http://")+"/u/test/app:1.2.3", registryClient, t.TempDir(), chartDownloaderOptions{})
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
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "download did not return after the context was canceled")
	}
}

func TestDownloadChart_ChartVersionLiteral(t *testing.T) {
	var tagListRequests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			tagListRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"name":"u/test/app","tags":["1.0.0"]}`)

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())

	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard), registry.ClientOptPlainHTTP())
	require.NoError(t, err)

	tests := []struct {
		name          string
		literal       bool
		expectTagList bool
	}{
		{name: "literal", literal: true},
		{name: "constraint", expectTagList: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagListRequests.Store(0)

			_, err := DownloadChart(context.Background(), "oci://"+strings.TrimPrefix(srv.URL, "http://")+"/u/test/app:1.x", registryClient, t.TempDir(), DownloadChartOptions{
				ChartVersionLiteral: tt.literal,
			})
			require.Error(t, err)
			assert.Equal(t, tt.expectTagList, tagListRequests.Load() > 0)
		})
	}
}
