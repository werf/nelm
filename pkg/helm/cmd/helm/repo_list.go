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

	"github.com/werf/nelm/pkg/helm/pkg/repo"
)

// Returns all repos from repos, except those with names matching ignoredRepoNames
// Inspired by https://stackoverflow.com/a/28701031/893211
func filterRepos(repos []*repo.Entry, ignoredRepoNames []string) []*repo.Entry {
	// if ignoredRepoNames is nil, just return repo
	if ignoredRepoNames == nil {
		return repos
	}

	filteredRepos := make([]*repo.Entry, 0)

	ignored := make(map[string]bool, len(ignoredRepoNames))
	for _, repoName := range ignoredRepoNames {
		ignored[repoName] = true
	}

	for _, repo := range repos {
		if _, removed := ignored[repo.Name]; !removed {
			filteredRepos = append(filteredRepos, repo)
		}
	}

	return filteredRepos
}

// Provide dynamic auto-completion for repo names
func compListRepos(_ string, ignoredRepoNames []string) []string {
	var rNames []string

	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err == nil && len(f.Repositories) > 0 {
		filteredRepos := filterRepos(f.Repositories, ignoredRepoNames)
		for _, repo := range filteredRepos {
			rNames = append(rNames, fmt.Sprintf("%s\t%s", repo.Name, repo.URL))
		}
	}
	return rNames
}
