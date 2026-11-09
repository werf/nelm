package registry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateReferenceTagCheckError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.WriteHeader(http.StatusOK)
		case "/v2/charts/test/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"name":"charts/test","tags":["1.0.0"]}`)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientOptPlainHTTP(), ClientOptWriter(io.Discard))
	require.NoError(t, err)

	repo := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/charts/test"

	tests := []struct {
		name          string
		tag           string
		expectedRef   string
		expectedError string
	}{
		{
			name:        "semver constraint falls back to tag list",
			tag:         "1.x",
			expectedRef: repo + ":1.0.0",
		},
		{
			name:          "non-constraint tag returns the error",
			tag:           "latest",
			expectedError: `check tag "latest"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := repo + ":" + tt.tag
			u, err := url.Parse(ref)
			require.NoError(t, err)

			_, resultURL, err := client.ValidateReference(ref, "", u)
			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedRef, resultURL.String())
		})
	}
}
