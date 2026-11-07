package action

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
)

func TestNewChartRepoRegistryClient_LoopbackPlainHTTP(t *testing.T) {
	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := newChartRepoRegistryClient(newChartPackTestContext(), common.ChartRepoConnectionOptions{}, filepath.Join(t.TempDir(), "config.json"))
	require.NoError(t, err)

	_, err = client.Resolve(strings.TrimPrefix(srv.URL, "http://") + "/u/test/app:1.2.3")
	require.Error(t, err)
	assert.Positive(t, requests.Load())
}
