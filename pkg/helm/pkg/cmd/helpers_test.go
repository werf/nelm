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
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/helm/intern/test"
	"github.com/werf/nelm/pkg/helm/pkg/action"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/pkg/helm/pkg/cli"
	kubefake "github.com/werf/nelm/pkg/helm/pkg/kube/fake"
	"github.com/werf/nelm/pkg/helm/pkg/release/common"
	release "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/pkg/helm/pkg/storage"
	"github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

func testTimestamper() time.Time { return time.Unix(242085845, 0).UTC() }

func init() {
	action.Timestamper = testTimestamper
}

func runTestCmd(t *testing.T, tests []cmdTestCase) {
	t.Helper()
	for _, tt := range tests {
		for i := 0; i <= tt.repeat; i++ {
			t.Run(tt.name, func(t *testing.T) {
				defer resetEnv()()

				storage := storageFixture()
				for _, rel := range tt.rels {
					if err := storage.Create(rel); err != nil {
						t.Fatal(err)
					}
				}
				t.Logf("running cmd (attempt %d): %s", i+1, tt.cmd)
				_, out, err := executeActionCommandC(storage, tt.cmd)
				if tt.wantError && err == nil {
					t.Errorf("expected error, got success with the following output:\n%s", out)
				}
				if !tt.wantError && err != nil {
					t.Errorf("expected no error, got: '%v'", err)
				}
				if tt.golden != "" {
					test.AssertGoldenString(t, out, tt.golden)
				}
			})
		}
	}
}

func storageFixture() *storage.Storage {
	return storage.Init(driver.NewMemory())
}

func executeActionCommandC(store *storage.Storage, cmd string) (*cobra.Command, string, error) {
	return executeActionCommandStdinC(store, nil, cmd)
}

func executeActionCommandStdinC(store *storage.Storage, in *os.File, cmd string) (*cobra.Command, string, error) {
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return nil, "", err
	}

	buf := new(bytes.Buffer)

	actionConfig := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartcommon.DefaultCapabilities,
	}

	root, err := newRootCmdWithConfig(actionConfig, buf, args, SetupLogging)
	if err != nil {
		return nil, "", err
	}

	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	oldStdin := os.Stdin
	defer func() {
		os.Stdin = oldStdin
	}()

	if in != nil {
		root.SetIn(in)
		os.Stdin = in
	}

	if mem, ok := store.Driver.(*driver.Memory); ok {
		mem.SetNamespace(settings.Namespace())
	}
	c, err := root.ExecuteC()

	result := buf.String()

	return c, result, err
}

// cmdTestCase describes a test case that works with releases.
type cmdTestCase struct {
	name      string
	cmd       string
	golden    string
	wantError bool
	// Rels are the available releases at the start of the test.
	rels []*release.Release
	// Number of repeats (in case a feature was previously flaky and the test checks
	// it's now stably producing identical results). 0 means test is run exactly once.
	repeat int
}

func executeActionCommand(cmd string) (*cobra.Command, string, error) {
	return executeActionCommandC(storageFixture(), cmd)
}

func resetEnv() func() {
	origEnv := os.Environ()
	return func() {
		os.Clearenv()
		for _, pair := range origEnv {
			kv := strings.SplitN(pair, "=", 2)
			os.Setenv(kv[0], kv[1])
		}
		settings = cli.New()
	}
}

func outputFlagCompletionTest(t *testing.T, cmdName string) {
	t.Helper()
	releasesMockWithStatus := func(info *release.Info, hooks ...*release.Hook) []*release.Release {
		info.LastDeployed = time.Unix(1452902400, 0).UTC()
		return []*release.Release{{
			Name:      "athos",
			Namespace: "default",
			Info:      info,
			Chart:     &chart.Chart{},
			Hooks:     hooks,
		}, {
			Name:      "porthos",
			Namespace: "default",
			Info:      info,
			Chart:     &chart.Chart{},
			Hooks:     hooks,
		}, {
			Name:      "aramis",
			Namespace: "default",
			Info:      info,
			Chart:     &chart.Chart{},
			Hooks:     hooks,
		}, {
			Name:      "dartagnan",
			Namespace: "gascony",
			Info:      info,
			Chart:     &chart.Chart{},
			Hooks:     hooks,
		}}
	}

	tests := []cmdTestCase{{
		name:   "completion for output flag long and before arg",
		cmd:    fmt.Sprintf("__complete %s --output ''", cmdName),
		golden: "output/output-comp.txt",
		rels: releasesMockWithStatus(&release.Info{
			Status: common.StatusDeployed,
		}),
	}, {
		name:   "completion for output flag long and after arg",
		cmd:    fmt.Sprintf("__complete %s aramis --output ''", cmdName),
		golden: "output/output-comp.txt",
		rels: releasesMockWithStatus(&release.Info{
			Status: common.StatusDeployed,
		}),
	}, {
		name:   "completion for output flag short and before arg",
		cmd:    fmt.Sprintf("__complete %s -o ''", cmdName),
		golden: "output/output-comp.txt",
		rels: releasesMockWithStatus(&release.Info{
			Status: common.StatusDeployed,
		}),
	}, {
		name:   "completion for output flag short and after arg",
		cmd:    fmt.Sprintf("__complete %s aramis -o ''", cmdName),
		golden: "output/output-comp.txt",
		rels: releasesMockWithStatus(&release.Info{
			Status: common.StatusDeployed,
		}),
	}, {
		name:   "completion for output flag, no filter",
		cmd:    fmt.Sprintf("__complete %s --output jso", cmdName),
		golden: "output/output-comp.txt",
		rels: releasesMockWithStatus(&release.Info{
			Status: common.StatusDeployed,
		}),
	}}
	runTestCmd(t, tests)
}
