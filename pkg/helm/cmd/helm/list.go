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

package helm

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/helm/pkg/action"
	"github.com/werf/nelm/pkg/helm/pkg/release"
)

// Returns all releases from 'releases', except those with names matching 'ignoredReleases'
func filterReleases(releases []*release.Release, ignoredReleaseNames []string) []*release.Release {
	// if ignoredReleaseNames is nil, just return releases
	if ignoredReleaseNames == nil {
		return releases
	}

	var filteredReleases []*release.Release
	for _, rel := range releases {
		found := false
		for _, ignoredName := range ignoredReleaseNames {
			if rel.Name == ignoredName {
				found = true
				break
			}
		}
		if !found {
			filteredReleases = append(filteredReleases, rel)
		}
	}

	return filteredReleases
}

// Provide dynamic auto-completion for release names
func compListReleases(toComplete string, ignoredReleaseNames []string, cfg *action.Configuration) ([]string, cobra.ShellCompDirective) {
	cobra.CompDebugln(fmt.Sprintf("compListReleases with toComplete %s", toComplete), settings.Debug)

	client := action.NewList(cfg)
	client.All = true
	client.Limit = 0
	// Do not filter so as to get the entire list of releases.
	// This will allow zsh and fish to match completion choices
	// on other criteria then prefix.  For example:
	//   helm status ingress<TAB>
	// can match
	//   helm status nginx-ingress
	//
	// client.Filter = fmt.Sprintf("^%s", toComplete)

	client.SetStateMask()
	releases, err := client.Run()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}

	var choices []string
	filteredReleases := filterReleases(releases, ignoredReleaseNames)
	for _, rel := range filteredReleases {
		choices = append(choices,
			fmt.Sprintf("%s\t%s-%s -> %s", rel.Name, rel.Chart.Metadata.Name, rel.Chart.Metadata.Version, rel.Info.Status.String()))
	}

	return choices, cobra.ShellCompDirectiveNoFileComp
}
