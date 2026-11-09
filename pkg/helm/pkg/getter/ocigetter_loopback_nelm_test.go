package getter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCIGetter_LoopbackPlainHTTP(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HELM_CONFIG_HOME", t.TempDir())

	g, err := NewOCIGetter()
	if err != nil {
		t.Fatal(err)
	}

	_, err = g.Get("oci://" + strings.TrimPrefix(srv.URL, "http://") + "/u/test/app:1.2.3")
	require.Error(t, err)
	assert.Positive(t, requests.Load())
}
