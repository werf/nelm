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

package postrenderer

import (
	"bytes"
	"fmt"

	"github.com/werf/nelm/pkg/helm/pkg/cli"
)

// PostRenderer is an interface different plugin runtimes
// it may be also be used without the factory for custom post-renderers
type PostRenderer interface {
	// Run expects a single buffer filled with Helm rendered manifests. It
	// expects the modified results to be returned on a separate buffer or an
	// error if there was an issue or failure while running the post render step
	Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error)
}

// NewPostRendererPlugin creates a PostRenderer that uses the plugin's Runtime
func NewPostRendererPlugin(_ *cli.EnvSettings, pluginName string, _ ...string) (PostRenderer, error) {
	return nil, fmt.Errorf("plugins are not supported, cannot use post-renderer plugin %q", pluginName)
}
