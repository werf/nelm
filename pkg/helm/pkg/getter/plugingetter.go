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

package getter

import (
	"github.com/werf/nelm/pkg/helm/pkg/cli"
)

// collectGetterPlugins scans for getter plugins.
// This will load plugins according to the cli.
func collectGetterPlugins(_ *cli.EnvSettings) (Providers, error) {
	return nil, nil
}
