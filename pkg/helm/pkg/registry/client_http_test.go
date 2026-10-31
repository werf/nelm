/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package registry

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"oras.land/oras-go/v2/content"
)

type HTTPRegistryClientTestSuite struct {
	TestRegistry
}

func (suite *HTTPRegistryClientTestSuite) SetupSuite() {
	// init test client
	setup(&suite.TestRegistry, false, false)
}

func (suite *HTTPRegistryClientTestSuite) TearDownSuite() {
	teardown(&suite.TestRegistry)
	_ = os.RemoveAll(suite.WorkspaceDir)
}

func (suite *HTTPRegistryClientTestSuite) Test_0_Login() {
	err := suite.RegistryClient.Login(suite.DockerRegistryHost,
		LoginOptBasicAuth("badverybad", "ohsobad"),
		LoginOptPlainText(true))
	suite.NotNil(err, "error logging into registry with bad credentials")

	err = suite.RegistryClient.Login(suite.DockerRegistryHost,
		LoginOptBasicAuth(testUsername, testPassword),
		LoginOptPlainText(true))
	suite.Nil(err, "no error logging into registry with good credentials")
}

func (suite *HTTPRegistryClientTestSuite) Test_1_Push() {
	testPush(&suite.TestRegistry)
}

func (suite *HTTPRegistryClientTestSuite) Test_2_Pull() {
	testPull(&suite.TestRegistry)
}

func (suite *HTTPRegistryClientTestSuite) Test_3_Tags() {
	testTags(&suite.TestRegistry)
}

func (suite *HTTPRegistryClientTestSuite) Test_4_ManInTheMiddle() {
	ref := suite.CompromisedRegistryHost + "/testrepo/supposedlysafechart:9.9.9"

	// returns content that does not match the expected digest
	_, err := suite.RegistryClient.Pull(ref)
	suite.NotNil(err)
	suite.True(errors.Is(err, content.ErrMismatchedDigest))
}

func (suite *HTTPRegistryClientTestSuite) Test_5_ImageIndex() {
	ref := suite.FakeRegistryHost + "/testrepo/image-index:0.1.0"

	_, err := suite.RegistryClient.Pull(ref)
	suite.Nil(err)
}

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

func TestHTTPRegistryClientTestSuite(t *testing.T) {
	suite.Run(t, new(HTTPRegistryClientTestSuite))
}
