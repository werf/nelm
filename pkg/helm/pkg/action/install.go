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
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"

	"github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/cli"
	"github.com/werf/nelm/pkg/helm/pkg/downloader"
	"github.com/werf/nelm/pkg/helm/pkg/getter"
	"github.com/werf/nelm/pkg/helm/pkg/registry"
	"github.com/werf/nelm/pkg/helm/pkg/repo"
)

// NOTESFILE_SUFFIX that we want to treat special. It goes through the templating engine
// but it's not a yaml file (resource) hence can't have hooks, etc. And the user actually
// wants to see this file after rendering in the status command. However, it must be a suffix
// since there can be filepath in front of it.
const notesFileSuffix = "NOTES.txt"

const defaultDirectoryPermission = 0o755

// ChartPathOptions captures common options used for controlling chart paths
type ChartPathOptions struct {
	CaFile                string // --ca-file
	CertFile              string // --cert-file
	KeyFile               string // --key-file
	InsecureSkipTLSverify bool   // --insecure-skip-verify
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

// write the <data> to <output-dir>/<name>. <append> controls if the file is created or content will be appended
func writeToFile(outputDir string, name string, data string, append bool) error {
	outfileName := strings.Join([]string{outputDir, name}, string(filepath.Separator))

	err := ensureDirectoryForFile(outfileName)
	if err != nil {
		return err
	}

	f, err := createOrOpenFile(outfileName, append)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("---\n# Source: %s\n%s\n", name, data))
	if err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", outfileName)
	return nil
}

func createOrOpenFile(filename string, append bool) (*os.File, error) {
	if append {
		return os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o600)
	}
	return os.Create(filename)
}

// check if the directory exists to create file. creates if don't exists
func ensureDirectoryForFile(file string) error {
	baseDir := path.Dir(file)
	_, err := os.Stat(baseDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(baseDir, defaultDirectoryPermission)
}

// TemplateName renders a name template, returning the name or an error.
func TemplateName(nameTemplate string) (string, error) {
	if nameTemplate == "" {
		return "", nil
	}

	t, err := template.New("name-template").Funcs(sprig.TxtFuncMap()).Parse(nameTemplate)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, nil); err != nil {
		return "", err
	}

	return b.String(), nil
}

// CheckDependencies checks the dependencies for a chart.
func CheckDependencies(ch *chart.Chart, reqs []*chart.Dependency) error {
	var missing []string

OUTER:
	for _, r := range reqs {
		for _, d := range ch.Dependencies() {
			if d.Name() == r.Name {
				continue OUTER
			}
		}
		missing = append(missing, r.Name)
	}

	if len(missing) > 0 {
		return errors.Errorf("found in Chart.yaml, but missing in charts/ directory: %s", strings.Join(missing, ", "))
	}
	return nil
}

// LocateChart looks for a chart directory in known places, and returns either the full path or an error.
//
// This does not ensure that the chart is well-formed; only that the requested filename exists.
//
// Order of resolution:
// - relative to current working directory
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

	if _, err := os.Stat(name); err == nil {
		abs, err := filepath.Abs(name)
		if err != nil {
			return abs, err
		}
		if c.Verify {
			if _, err := downloader.VerifyChart(abs, c.Keyring); err != nil {
				return "", err
			}
		}
		return abs, nil
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, errors.Errorf("path %q not found", name)
	}

	dl := downloader.ChartDownloader{
		Out:     os.Stdout,
		Keyring: c.Keyring,
		Getters: getter.All(settings),
		Options: []getter.Option{
			getter.WithPassCredentialsAll(c.PassCredentialsAll),
			getter.WithTLSClientConfig(c.CertFile, c.KeyFile, c.CaFile),
			getter.WithInsecureSkipVerifyTLS(c.InsecureSkipTLSverify),
			getter.WithPlainHTTP(c.PlainHTTP),
		},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		RegistryClient:   c.registryClient,
	}

	if registry.IsOCI(name) {
		dl.Options = append(dl.Options, getter.WithRegistryClient(c.registryClient))
	}

	if c.Verify {
		dl.Verify = downloader.VerifyAlways
	}
	if c.RepoURL != "" {
		chartURL, err := repo.FindChartInAuthAndTLSAndPassRepoURL(c.RepoURL, c.Username, c.Password, name, version,
			c.CertFile, c.KeyFile, c.CaFile, c.InsecureSkipTLSverify, c.PassCredentialsAll, getter.All(settings))
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
		if c.PassCredentialsAll || (u1.Scheme == u2.Scheme && u1.Host == u2.Host) {
			dl.Options = append(dl.Options, getter.WithBasicAuth(c.Username, c.Password))
		} else {
			dl.Options = append(dl.Options, getter.WithBasicAuth("", ""))
		}
	} else {
		dl.Options = append(dl.Options, getter.WithBasicAuth(c.Username, c.Password))
	}

	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return "", err
	}

	filename, _, err := dl.DownloadTo(name, version, settings.RepositoryCache)
	if err != nil {
		return "", err
	}

	lname, err := filepath.Abs(filename)
	if err != nil {
		return filename, err
	}
	return lname, nil
}

func (c *ChartPathOptions) SetRegistryClient(cli *registry.Client) {
	c.registryClient = cli
}
