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

package lint // import "helm.sh/helm/v3/pkg/lint"

import (
	"path/filepath"

	"github.com/werf/nelm/pkg/helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/helm/pkg/lint/rules"
	"github.com/werf/nelm/pkg/helm/pkg/lint/support"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
)

// All runs all of the available linters on the given base directory.
func All(basedir string, values map[string]interface{}, namespace string, _ bool, opts helmopts.HelmOptions) support.Linter {
	return AllWithKubeVersion(basedir, values, namespace, nil, opts)
}

// AllWithKubeVersion runs all the available linters on the given base directory, allowing to specify the kubernetes version.
func AllWithKubeVersion(basedir string, values map[string]interface{}, namespace string, kubeVersion *chartutil.KubeVersion, opts helmopts.HelmOptions) support.Linter {
	// Using abs path to get directory context
	chartDir, _ := filepath.Abs(basedir)

	linter := support.Linter{ChartDir: chartDir}
	if false {
		rules.Chartfile(&linter)
	}
	rules.ValuesWithOverrides(&linter, values)
	rules.TemplatesWithKubeVersion(&linter, values, namespace, kubeVersion, opts)
	rules.Dependencies(&linter, opts)
	return linter
}
