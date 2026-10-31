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
	"os"
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

func TestHTTPRegistryClientTestSuite(t *testing.T) {
	suite.Run(t, new(HTTPRegistryClientTestSuite))
}
