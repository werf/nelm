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

package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/werf/nelm/pkg/helm/pkg/action"
	"github.com/werf/nelm/pkg/helm/pkg/chart/common"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	kubefake "github.com/werf/nelm/pkg/helm/pkg/kube/fake"
	rcommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	release "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

func TestReleaseTestingCompletion(t *testing.T) {
	checkReleaseCompletion(t, "test", false)
}

func TestReleaseTestingFileCompletion(t *testing.T) {
	checkFileCompletion(t, "test", false)
	checkFileCompletion(t, "test myrelease", false)
}

func TestReleaseTestNotesHandling(t *testing.T) {
	// Test that ensures notes behavior is correct for test command
	// This is a simpler test that focuses on the core functionality

	rel := &release.Release{
		Name:      "test-release",
		Namespace: "default",
		Info: &release.Info{
			Status: rcommon.StatusDeployed,
			Notes:  "Some important notes that should be hidden by default",
		},
		Chart: &chart.Chart{Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"}},
	}

	// Set up storage
	store := storageFixture()
	store.Create(rel)

	// Set up action configuration properly
	actionConfig := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.FailingKubeClient{PrintingKubeClient: kubefake.PrintingKubeClient{Out: io.Discard}},
		Capabilities: common.DefaultCapabilities,
	}

	// Test the newReleaseTestCmd function directly
	var buf1 bytes.Buffer

	// Test 1: Default behavior (should hide notes)
	cmd1 := newReleaseTestCmd(actionConfig, &buf1)
	cmd1.SetArgs([]string{"test-release"})
	err1 := cmd1.Execute()
	if err1 != nil {
		t.Fatalf("Unexpected error for default test: %v", err1)
	}
	output1 := buf1.String()
	if strings.Contains(output1, "NOTES:") {
		t.Errorf("Expected notes to be hidden by default, but found NOTES section in output: %s", output1)
	}
}
