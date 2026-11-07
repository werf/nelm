package registry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func (suite *HTTPRegistryClientTestSuite) Test_6_PushExtraTags() {
	chartData, err := os.ReadFile("../downloader/testdata/local-subchart-0.1.0.tgz")
	suite.Require().NoError(err)
	meta, err := extractChartMeta(chartData)
	suite.Require().NoError(err)

	repo := fmt.Sprintf("%s/extratags/%s", suite.DockerRegistryHost, meta.Name)

	tests := []struct {
		name         string
		extraTags    []string
		expectedTags []string
		expectError  bool
	}{
		{
			name:         "literal and duplicate tags",
			extraTags:    []string{"latest", "v0.1.0", meta.Version, "latest"},
			expectedTags: []string{meta.Version, "latest", "v0.1.0"},
		},
		{
			name:         "plus sign is encoded",
			extraTags:    []string{"0.1.0+build"},
			expectedTags: []string{meta.Version, "0.1.0_build"},
		},
		{
			name:        "invalid tag",
			extraTags:   []string{"bad tag"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result, err := suite.RegistryClient.Push(chartData, fmt.Sprintf("%s:%s", repo, meta.Version), PushOptExtraTags(tt.extraTags))
			if tt.expectError {
				suite.Require().Error(err)
				return
			}
			suite.Require().NoError(err)

			for _, tag := range tt.expectedTags {
				desc, err := suite.RegistryClient.Resolve(fmt.Sprintf("%s:%s", repo, tag))
				suite.Require().NoError(err, tag)
				suite.Equal(result.Manifest.Digest, desc.Digest.String(), tag)
			}
		})
	}
}

func (suite *HTTPRegistryClientTestSuite) Test_7_ValidateReferenceNonSemverTag() {
	chartData, err := os.ReadFile("../downloader/testdata/local-subchart-0.1.0.tgz")
	suite.Require().NoError(err)
	meta, err := extractChartMeta(chartData)
	suite.Require().NoError(err)

	repo := fmt.Sprintf("%s/nonsemver/%s", suite.DockerRegistryHost, meta.Name)
	_, err = suite.RegistryClient.Push(chartData, fmt.Sprintf("%s:%s", repo, meta.Version), PushOptExtraTags([]string{"latest", "1.x"}))
	suite.Require().NoError(err)

	tests := []struct {
		name        string
		ref         string
		version     string
		expectedRef string
		expectError bool
	}{
		{
			name:        "non-semver tag in reference",
			ref:         "oci://" + repo + ":latest",
			expectedRef: "oci://" + repo + ":latest",
		},
		{
			name:        "non-semver tag as version",
			ref:         "oci://" + repo,
			version:     "latest",
			expectedRef: "oci://" + repo + ":latest",
		},
		{
			name:        "existing tag that is also a constraint",
			ref:         "oci://" + repo + ":1.x",
			expectedRef: "oci://" + repo + ":1.x",
		},
		{
			name:        "semver constraint",
			ref:         "oci://" + repo,
			version:     "^0.1.0",
			expectedRef: "oci://" + repo + ":" + meta.Version,
		},
		{
			name:        "no version",
			ref:         "oci://" + repo,
			expectedRef: "oci://" + repo + ":" + meta.Version,
		},
		{
			name:        "missing non-semver tag",
			ref:         "oci://" + repo + ":missing",
			expectError: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			u, err := url.Parse(tt.ref)
			suite.Require().NoError(err)

			_, resultURL, err := suite.RegistryClient.ValidateReference(tt.ref, tt.version, u)
			if tt.expectError {
				suite.Require().Error(err)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(tt.expectedRef, resultURL.String())

			result, err := suite.RegistryClient.Pull(resultURL.String())
			suite.Require().NoError(err)
			suite.Equal(meta.Version, result.Chart.Meta.Version)
		})
	}
}

func (suite *HTTPRegistryClientTestSuite) Test_8_PushExtraTagFailure() {
	chartData, err := os.ReadFile("../downloader/testdata/local-subchart-0.1.0.tgz")
	suite.Require().NoError(err)
	meta, err := extractChartMeta(chartData)
	suite.Require().NoError(err)

	target, err := url.Parse("http://" + suite.DockerRegistryHost)
	suite.Require().NoError(err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/manifests/latest") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.SetBasicAuth(testUsername, testPassword)
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()

	var out strings.Builder
	client, err := NewClient(ClientOptPlainHTTP(), ClientOptWriter(&out))
	suite.Require().NoError(err)

	ref := fmt.Sprintf("%s/failingtag/%s:%s", strings.TrimPrefix(server.URL, "http://"), meta.Name, meta.Version)
	_, err = client.Push(chartData, ref, PushOptExtraTags([]string{"latest"}))
	suite.Require().ErrorContains(err, fmt.Sprintf("chart pushed as %s, but tagging it also as \"latest\" failed", ref))
	suite.Contains(out.String(), "Pushed: "+ref)

	_, err = client.Pull(ref)
	suite.Require().NoError(err)
}

func (suite *HTTPRegistryClientTestSuite) Test_9_PushProvTags() {
	chartData, err := os.ReadFile("../downloader/testdata/local-subchart-0.1.0.tgz")
	suite.Require().NoError(err)
	meta, err := extractChartMeta(chartData)
	suite.Require().NoError(err)

	tests := []struct {
		name          string
		tag           string
		extraTags     []string
		expectedError string
	}{
		{
			name:          "main tag ending in .prov",
			tag:           "0.1.0.prov",
			expectedError: `tag "0.1.0.prov" is not allowed: tags ending in ".prov" address provenance files`,
		},
		{
			name:          "extra tag ending in .prov",
			tag:           meta.Version,
			extraTags:     []string{"latest", "latest.prov"},
			expectedError: `tag "latest.prov" is not allowed: tags ending in ".prov" address provenance files`,
		},
		{
			name:      "tags containing prov elsewhere",
			tag:       meta.Version,
			extraTags: []string{"prov", "x.prov1"},
		},
	}

	for i, tt := range tests {
		suite.Run(tt.name, func() {
			repo := fmt.Sprintf("%s/provtags%d/%s", suite.DockerRegistryHost, i, meta.Name)
			_, err := suite.RegistryClient.Push(chartData, fmt.Sprintf("%s:%s", repo, tt.tag), PushOptStrictMode(false), PushOptExtraTags(tt.extraTags))
			if tt.expectedError != "" {
				suite.Require().EqualError(err, tt.expectedError)

				_, err = suite.RegistryClient.Resolve(fmt.Sprintf("%s:%s", repo, tt.tag))
				suite.Require().Error(err, "nothing must be pushed")

				return
			}
			suite.Require().NoError(err)

			for _, tag := range append([]string{tt.tag}, tt.extraTags...) {
				_, err := suite.RegistryClient.Resolve(fmt.Sprintf("%s:%s", repo, tag))
				suite.Require().NoError(err, tag)
			}
		})
	}
}
