package getter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

func TestGetterWithContext_CancelsBlockedRequest(t *testing.T) {
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

	tests := []struct {
		name   string
		getter func() (Getter, error)
		href   string
	}{
		{
			name:   "OCI",
			getter: func() (Getter, error) { return NewOCIGetter(WithRegistryClient(registryClient)) },
			href:   "oci://" + strings.TrimPrefix(srv.URL, "http://") + "/u/test/chart:0.1.0",
		},
		{
			name:   "HTTP",
			getter: func() (Getter, error) { return NewHTTPGetter() },
			href:   srv.URL + "/chart-0.1.0.tgz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := tt.getter()
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				_, err := g.Get(tt.href, WithContext(ctx))
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
				require.FailNow(t, "Get did not return after the context was cancelled")
			}
		})
	}
}
