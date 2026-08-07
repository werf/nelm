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

package action

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	ci "github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/cli"
	"github.com/werf/nelm/pkg/helm/pkg/downloader"
	"github.com/werf/nelm/pkg/helm/pkg/getter"
	"github.com/werf/nelm/pkg/helm/pkg/registry"
	ri "github.com/werf/nelm/pkg/helm/pkg/release"
	release "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/pkg/helm/pkg/repo/v1"
)

// notesFileSuffix that we want to treat specially. It goes through the templating engine
// but it's not a YAML file (resource) hence can't have hooks, etc. And the user actually
// wants to see this file after rendering in the status command. However, it must be a suffix
// since there can be filepath in front of it.
const notesFileSuffix = "NOTES.txt"

const defaultDirectoryPermission = 0755

// ChartPathOptions captures common options used for controlling chart paths
type ChartPathOptions struct {
	CaFile                string // --ca-file
	CertFile              string // --cert-file
	KeyFile               string // --key-file
	InsecureSkipTLSVerify bool   // --insecure-skip-verify
	PlainHTTP             bool   // --plain-http
	Keyring               string // --keyring
	Password              string // --password
	PassCredentialsAll    bool   // --pass-credentials
	RepoURL               string // --repo
	Username              string // --username
	Verify                bool   // --verify
	Version               string // --version

	// registryClient provides a registry client but is not added with
	// options from a flag
	registryClient *registry.Client
}

func releaseListToV1List(ls []ri.Releaser) ([]*release.Release, error) {
	rls := make([]*release.Release, 0, len(ls))
	for _, val := range ls {
		rel, err := releaserToV1Release(val)
		if err != nil {
			return nil, err
		}
		rls = append(rls, rel)
	}

	return rls, nil
}

func releaseV1ListToReleaserList(ls []*release.Release) ([]ri.Releaser, error) {
	rls := make([]ri.Releaser, 0, len(ls))
	for _, val := range ls {
		rls = append(rls, val)
	}

	return rls, nil
}

// write the <data> to <output-dir>/<name>. <appendData> controls if the file is created or content will be appended
func writeToFile(outputDir string, name string, data string, appendData bool) error {
	outfileName := strings.Join([]string{outputDir, name}, string(filepath.Separator))

	err := ensureDirectoryForFile(outfileName)
	if err != nil {
		return err
	}

	f, err := createOrOpenFile(outfileName, appendData)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = fmt.Fprintf(f, "---\n# Source: %s\n%s\n", name, data)

	if err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", outfileName)
	return nil
}

func createOrOpenFile(filename string, appendData bool) (*os.File, error) {
	if appendData {
		return os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	}
	return os.Create(filename)
}

// check if the directory exists to create file. creates if doesn't exist
func ensureDirectoryForFile(file string) error {
	baseDir := filepath.Dir(file)
	_, err := os.Stat(baseDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return os.MkdirAll(baseDir, defaultDirectoryPermission)
}

// CheckDependencies checks the dependencies for a chart.
func CheckDependencies(ch ci.Charter, reqs []ci.Dependency) error {
	ac, err := ci.NewAccessor(ch)
	if err != nil {
		return err
	}

	var missing []string

OUTER:
	for _, r := range reqs {
		rac, err := ci.NewDependencyAccessor(r)
		if err != nil {
			return err
		}
		for _, d := range ac.Dependencies() {
			dac, err := ci.NewAccessor(d)
			if err != nil {
				return err
			}
			if dac.Name() == rac.Name() {
				continue OUTER
			}
		}
		missing = append(missing, rac.Name())
	}

	if len(missing) > 0 {
		return fmt.Errorf("found in Chart.yaml, but missing in charts/ directory: %s", strings.Join(missing, ", "))
	}
	return nil
}

func portOrDefault(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}

	switch u.Scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func urlEqual(u1, u2 *url.URL) bool {
	return u1.Scheme == u2.Scheme && u1.Hostname() == u2.Hostname() && portOrDefault(u1) == portOrDefault(u2)
}

// LocateChart looks for a chart directory in known places, and returns either the full path or an error.
//
// This does not ensure that the chart is well-formed; only that the requested filename exists.
//
// Order of resolution:
// - relative to current working directory when --repo flag is not presented
// - if path is absolute or begins with '.', error out here
// - URL
//
// If 'verify' was set on ChartPathOptions, this will attempt to also verify the chart.
func (c *ChartPathOptions) LocateChart(name string, settings *cli.EnvSettings) (string, error) {
	if registry.IsOCI(name) && c.registryClient == nil {
		return "", fmt.Errorf("unable to lookup chart %q, missing registry client", name)
	}

	name = strings.TrimSpace(name)
	version := strings.TrimSpace(c.Version)

	if c.RepoURL == "" {
		if _, err := os.Stat(name); err == nil {
			abs, err := filepath.Abs(name)
			if err != nil {
				return abs, err
			}
			if c.Verify {
				if _, err := downloader.VerifyChart(abs, abs+".prov", c.Keyring); err != nil {
					return "", err
				}
			}
			return abs, nil
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
			return name, fmt.Errorf("path %q not found", name)
		}
	}

	dl := downloader.ChartDownloader{
		Out:     os.Stdout,
		Keyring: c.Keyring,
		Getters: getter.All(settings),
		Options: []getter.Option{
			getter.WithPassCredentialsAll(c.PassCredentialsAll),
			getter.WithTLSClientConfig(c.CertFile, c.KeyFile, c.CaFile),
			getter.WithInsecureSkipVerifyTLS(c.InsecureSkipTLSVerify),
			getter.WithPlainHTTP(c.PlainHTTP),
			getter.WithBasicAuth(c.Username, c.Password),
		},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		ContentCache:     settings.ContentCache,
		RegistryClient:   c.registryClient,
	}

	if registry.IsOCI(name) {
		dl.Options = append(dl.Options, getter.WithRegistryClient(c.registryClient))
	}

	if c.Verify {
		dl.Verify = downloader.VerifyAlways
	}
	if c.RepoURL != "" {
		chartURL, err := repo.FindChartInRepoURL(
			c.RepoURL,
			name,
			getter.All(settings),
			repo.WithChartVersion(version),
			repo.WithClientTLS(c.CertFile, c.KeyFile, c.CaFile),
			repo.WithUsernamePassword(c.Username, c.Password),
			repo.WithInsecureSkipTLSVerify(c.InsecureSkipTLSVerify),
			repo.WithPassCredentialsAll(c.PassCredentialsAll),
		)
		if err != nil {
			return "", err
		}
		name = chartURL

		// Only pass the user/pass on when the user has said to or when the
		// location of the chart repo and the chart are the same domain.
		u1, err := url.Parse(c.RepoURL)
		if err != nil {
			return "", err
		}
		u2, err := url.Parse(chartURL)
		if err != nil {
			return "", err
		}

		// Host on URL (returned from url.Parse) contains the port if present.
		// This check ensures credentials are not passed between different
		// services on different ports.
		if c.PassCredentialsAll || urlEqual(u1, u2) {
			dl.Options = append(dl.Options, getter.WithBasicAuth(c.Username, c.Password))
		} else {
			dl.Options = append(dl.Options, getter.WithBasicAuth("", ""))
		}
	} else {
		dl.Options = append(dl.Options, getter.WithBasicAuth(c.Username, c.Password))
	}

	if err := os.MkdirAll(settings.RepositoryCache, 0755); err != nil {
		return "", err
	}

	filename, _, err := dl.DownloadToCache(name, version)
	if err != nil {
		return "", err
	}

	lname, err := filepath.Abs(filename)
	if err != nil {
		return filename, err
	}
	return lname, nil
}
