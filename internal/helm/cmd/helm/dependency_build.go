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

package helm_v3

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/util/homedir"

	"github.com/werf/nelm/internal/helm/cmd/helm/require"
	"github.com/werf/nelm/internal/helm/pkg/action"
	"github.com/werf/nelm/internal/helm/pkg/downloader"
	"github.com/werf/nelm/internal/helm/pkg/getter"
	"github.com/werf/nelm/internal/helm/pkg/werf/helmopts"
)

const dependencyBuildDesc = `
Build out the charts/ directory from the Chart.lock file.

Build is used to reconstruct a chart's dependencies to the state specified in
the lock file. This will not re-negotiate dependencies, as 'helm dependency update'
does.

If no lock file is found, 'helm dependency build' will mirror the behavior
of 'helm dependency update'.
`

func newDependencyBuildCmd(cfg *action.Configuration, out io.Writer) *cobra.Command {
	client := action.NewDependency()

	cmd := &cobra.Command{
		Use:   "build CHART",
		Short: "rebuild the charts/ directory based on the Chart.lock file",
		Long:  dependencyBuildDesc,
		Args:  require.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chartpath := "."
			if len(args) > 0 {
				chartpath = filepath.Clean(args[0])
			}
			man := &downloader.Manager{
				Out:              out,
				ChartPath:        chartpath,
				Keyring:          client.Keyring,
				SkipUpdate:       client.SkipRefresh,
				Getters:          getter.All(settings),
				RegistryClient:   cfg.RegistryClient,
				RepositoryConfig: settings.RepositoryConfig,
				RepositoryCache:  settings.RepositoryCache,
				Debug:            settings.Debug,
			}
			if client.Verify {
				man.Verify = downloader.VerifyIfPossible
			}

			opts := helmopts.HelmOptions{
				ChartLoadOpts: helmopts.ChartLoadOptions{
					DepDownloader: man,
					NoSecrets:     true,
				},
			}

			err := man.Build(opts)
			if e, ok := err.(downloader.ErrRepoNotFound); ok {
				return fmt.Errorf("%s. Please add the missing repos via 'helm repo add'", e.Error())
			}
			return err
		},
	}

	f := cmd.Flags()
	f.BoolVar(&client.Verify, "verify", false, "verify the packages against signatures")
	f.StringVar(&client.Keyring, "keyring", defaultKeyring(), "keyring containing public keys")
	f.BoolVar(&client.SkipRefresh, "skip-refresh", false, "do not refresh the local repository cache")

	return cmd
}

// defaultKeyring returns the expanded path to the default keyring.
func defaultKeyring() string {
	if v, ok := os.LookupEnv("GNUPGHOME"); ok {
		return filepath.Join(v, "pubring.gpg")
	}
	return filepath.Join(homedir.HomeDir(), ".gnupg", "pubring.gpg")
}
