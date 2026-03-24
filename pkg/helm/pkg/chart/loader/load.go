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

package loader

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/werf/chartextender"
	"github.com/werf/nelm/pkg/helm/pkg/werf/file"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
	"github.com/werf/nelm/pkg/helm/pkg/werf/secrets"
	"github.com/werf/nelm/pkg/helm/pkg/werf/secrets/runtimedata"
)

// ChartLoader loads a chart.
type ChartLoader interface {
	Load(options helmopts.HelmOptions) (*chart.Chart, error)
}

// Loader returns a new ChartLoader appropriate for the given chart name
func Loader(name string) (ChartLoader, error) {
	isDir, err := loader(name)
	if err != nil {
		return nil, errors.Wrapf(err, "error checking if %s is a directory", name)
	}

	if isDir {
		return DirLoader(name), nil
	}
	return FileLoader(name), nil
}

func loader(name string) (bool, error) {
	if file.ChartFileReader == nil {
		fi, err := os.Stat(name)
		if err != nil {
			return false, err
		}
		if fi.IsDir() {
			return true, nil
		}
		return false, nil
	}
	return file.ChartFileReader.ChartIsDir(name)
}

// Load takes a string name, tries to resolve it to a file or directory, and then loads it.
//
// This is the preferred way to load a chart. It will discover the chart encoding
// and hand off to the appropriate chart reader.
//
// If a .helmignore file is present, the directory loader will skip loading any files
// matching it.
func Load(name string, opts helmopts.HelmOptions) (*chart.Chart, error) {
	l, err := Loader(name)
	if err != nil {
		return nil, err
	}
	return l.Load(opts)
}

// BufferedFile represents an archive file buffered for later processing.
type BufferedFile struct {
	Name string
	Data []byte
}

// LoadFiles loads from in-memory files.
func LoadFiles(files []*BufferedFile, opts helmopts.HelmOptions) (*chart.Chart, error) {
	c := new(chart.Chart)
	subcharts := make(map[string][]*BufferedFile)

	c.SecretsRuntimeData = secrets.NewSecretsRuntimeData()

	// do not rely on assumed ordering of files in the chart and crash
	// if Chart.yaml was not coming early enough to initialize metadata
	for _, f := range files {
		c.Raw = append(c.Raw, &chart.File{Name: f.Name, Data: f.Data})
		if f.Name == "Chart.yaml" {
			if c.Metadata == nil {
				c.Metadata = new(chart.Metadata)
			}
			if err := yaml.Unmarshal(f.Data, c.Metadata); err != nil {
				return c, errors.Wrap(err, "cannot load Chart.yaml")
			}
			// NOTE(bacongobbler): while the chart specification says that APIVersion must be set,
			// Helm 2 accepted charts that did not provide an APIVersion in their chart metadata.
			// Because of that, if APIVersion is unset, we should assume we're loading a v1 chart.
			if c.Metadata.APIVersion == "" {
				c.Metadata.APIVersion = chart.APIVersionV1
			}
		}
	}
	for _, f := range files {
		switch {
		case f.Name == "Chart.yaml":
			// already processed
			continue
		case f.Name == "Chart.lock":
			c.Lock = new(chart.Lock)
			if err := yaml.Unmarshal(f.Data, &c.Lock); err != nil {
				return c, errors.Wrap(err, "cannot load Chart.lock")
			}
		case f.Name == "values.yaml":
			c.Values = make(map[string]interface{})
			if err := yaml.Unmarshal(f.Data, &c.Values); err != nil {
				return c, errors.Wrap(err, "cannot load values.yaml")
			}
		case f.Name == "values.schema.json":
			c.Schema = f.Data

		// Deprecated: requirements.yaml is deprecated use Chart.yaml.
		// We will handle it for you because we are nice people
		case f.Name == "requirements.yaml":
			if c.Metadata == nil {
				c.Metadata = new(chart.Metadata)
			}
			if c.Metadata.APIVersion != chart.APIVersionV1 {
				log.Printf("Warning: Dependencies are handled in Chart.yaml since apiVersion \"v2\". We recommend migrating dependencies to Chart.yaml.")
			}
			if err := yaml.Unmarshal(f.Data, c.Metadata); err != nil {
				return c, errors.Wrap(err, "cannot load requirements.yaml")
			}
			if c.Metadata.APIVersion == chart.APIVersionV1 {
				c.Files = append(c.Files, &chart.File{Name: f.Name, Data: f.Data})
			}
		// Deprecated: requirements.lock is deprecated use Chart.lock.
		case f.Name == "requirements.lock":
			c.Lock = new(chart.Lock)
			if err := yaml.Unmarshal(f.Data, &c.Lock); err != nil {
				return c, errors.Wrap(err, "cannot load requirements.lock")
			}
			if c.Metadata == nil {
				c.Metadata = new(chart.Metadata)
			}
			if c.Metadata.APIVersion == chart.APIVersionV1 {
				c.Files = append(c.Files, &chart.File{Name: f.Name, Data: f.Data})
			}

		case strings.HasPrefix(f.Name, "templates/"):
			c.Templates = append(c.Templates, &chart.File{Name: f.Name, Data: f.Data})
		case strings.HasPrefix(f.Name, "charts/"):
			if filepath.Ext(f.Name) == ".prov" {
				c.Files = append(c.Files, &chart.File{Name: f.Name, Data: f.Data})
				continue
			}

			fname := strings.TrimPrefix(f.Name, "charts/")
			cname := strings.SplitN(fname, "/", 2)[0]
			subcharts[cname] = append(subcharts[cname], &BufferedFile{Name: fname, Data: f.Data})
		case strings.HasPrefix(f.Name, "ts/node_modules/"):
			c.RuntimeDepsFiles = append(c.RuntimeDepsFiles, &chart.File{Name: f.Name, Data: f.Data})
		case strings.HasPrefix(f.Name, "ts/"):
			c.RuntimeFiles = append(c.RuntimeFiles, &chart.File{Name: f.Name, Data: f.Data})
		default:
			c.Files = append(c.Files, &chart.File{Name: f.Name, Data: f.Data})
		}
	}

	switch opts.ChartLoadOpts.ChartType {
	case helmopts.ChartTypeBundle:
		c.ExtraValues = opts.ChartLoadOpts.ExtraValues

		if !opts.ChartLoadOpts.NoSecrets {
			if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
				context.Background(),
				convertBufferedFilesForChartExtender(files),
				secrets_manager.Manager,
				runtimedata.DecodeAndLoadSecretsOptions{
					CustomSecretValueFiles:     opts.ChartLoadOpts.SecretValuesFiles,
					LoadFromLocalFilesystem:    true,
					NoDecryptSecrets:           opts.ChartLoadOpts.SecretKeyIgnore,
					SecretsWorkingDir:          opts.ChartLoadOpts.SecretWorkDir,
					WithoutDefaultSecretValues: opts.ChartLoadOpts.DefaultSecretValuesDisable,
				},
			); err != nil {
				return nil, fmt.Errorf("error decoding secrets: %w", err)
			}
		}

		if opts.ChartLoadOpts.DefaultValuesDisable {
			c.Values = nil
		}
	case helmopts.ChartTypeChart:
		c.ExtraValues = opts.ChartLoadOpts.ExtraValues

		if !opts.ChartLoadOpts.NoSecrets {
			if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
				context.Background(),
				convertBufferedFilesForChartExtender(files),
				secrets_manager.Manager,
				runtimedata.DecodeAndLoadSecretsOptions{
					CustomSecretValueFiles:     opts.ChartLoadOpts.SecretValuesFiles,
					LoadFromLocalFilesystem:    file.ChartFileReader == nil,
					NoDecryptSecrets:           opts.ChartLoadOpts.SecretKeyIgnore,
					SecretsWorkingDir:          opts.ChartLoadOpts.SecretWorkDir,
					WithoutDefaultSecretValues: opts.ChartLoadOpts.DefaultSecretValuesDisable,
				},
			); err != nil {
				return nil, fmt.Errorf("error decoding secrets: %w", err)
			}
		}

		c.Metadata = chartextender.AutosetChartMetadata(
			c.Metadata,
			chartextender.GetHelmChartMetadataOptions{
				DefaultAPIVersion:  opts.ChartLoadOpts.DefaultChartAPIVersion,
				DefaultName:        opts.ChartLoadOpts.DefaultChartName,
				DefaultVersion:     opts.ChartLoadOpts.DefaultChartVersion,
				OverrideAppVersion: opts.ChartLoadOpts.ChartAppVersion,
			},
		)

		c.Templates = append(c.Templates, &chart.File{
			Name: "templates/_werf_helpers.tpl",
		})

		if opts.ChartLoadOpts.DefaultValuesDisable {
			c.Values = nil
		}
	case helmopts.ChartTypeSubchart:
		if !opts.ChartLoadOpts.NoSecrets {
			if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
				context.Background(),
				convertBufferedFilesForChartExtender(files),
				secrets_manager.Manager,
				runtimedata.DecodeAndLoadSecretsOptions{
					LoadFromLocalFilesystem:    file.ChartFileReader == nil,
					NoDecryptSecrets:           opts.ChartLoadOpts.SecretKeyIgnore,
					SecretsWorkingDir:          opts.ChartLoadOpts.SecretWorkDir,
					WithoutDefaultSecretValues: opts.ChartLoadOpts.DefaultSecretValuesDisable,
				},
			); err != nil {
				return nil, fmt.Errorf("error decoding secrets: %w", err)
			}
		}
	case helmopts.ChartTypeChartStub:
		if !opts.ChartLoadOpts.NoSecrets {
			if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
				context.Background(),
				convertBufferedFilesForChartExtender(files),
				secrets_manager.Manager,
				runtimedata.DecodeAndLoadSecretsOptions{
					LoadFromLocalFilesystem:    true,
					NoDecryptSecrets:           opts.ChartLoadOpts.SecretKeyIgnore,
					SecretsWorkingDir:          opts.ChartLoadOpts.SecretWorkDir,
					WithoutDefaultSecretValues: opts.ChartLoadOpts.DefaultSecretValuesDisable,
				},
			); err != nil {
				return nil, fmt.Errorf("error decoding secrets: %w", err)
			}
		}

		c.Metadata = chartextender.AutosetChartMetadata(
			c.Metadata,
			chartextender.GetHelmChartMetadataOptions{
				DefaultAPIVersion: chart.APIVersionV2,
				DefaultName:       "stubchartname",
				DefaultVersion:    "1.0.0",
			},
		)

		c.Templates = append(c.Templates, &chart.File{
			Name: "templates/_werf_helpers.tpl",
		})
	default:
		panic("unexpected type")
	}

	if c.Metadata == nil {
		return c, errors.New("Chart.yaml file is missing")
	}

	if err := c.Validate(); err != nil {
		return c, err
	}

	for n, files := range subcharts {
		var sc *chart.Chart
		var err error
		switch {
		case strings.IndexAny(n, "_.") == 0:
			continue
		case filepath.Ext(n) == ".tgz":
			file := files[0]
			if file.Name != n {
				return c, errors.Errorf("error unpacking tar in %s: expected %s, got %s", c.Name(), n, file.Name)
			}

			opts.ChartLoadOpts.ChartType = helmopts.ChartTypeSubchart

			// Untar the chart and add to c.Dependencies
			sc, err = LoadArchive(bytes.NewBuffer(file.Data), opts)
		default:
			// We have to trim the prefix off of every file, and ignore any file
			// that is in charts/, but isn't actually a chart.
			buff := make([]*BufferedFile, 0, len(files))
			for _, f := range files {
				parts := strings.SplitN(f.Name, "/", 2)
				if len(parts) < 2 {
					continue
				}
				f.Name = parts[1]
				buff = append(buff, f)
			}

			opts.ChartLoadOpts.ChartType = helmopts.ChartTypeSubchart

			sc, err = LoadFiles(buff, opts)
		}

		if err != nil {
			return c, errors.Wrapf(err, "error unpacking %s in %s", n, c.Name())
		}
		c.AddDependency(sc)
	}

	return c, nil
}

func convertBufferedFilesForChartExtender(files []*BufferedFile) []*file.ChartExtenderBufferedFile {
	var res []*file.ChartExtenderBufferedFile
	for _, f := range files {
		f1 := new(file.ChartExtenderBufferedFile)
		*f1 = file.ChartExtenderBufferedFile(*f)
		res = append(res, f1)
	}
	return res
}

func convertChartExtenderFilesToBufferedFiles(files []*file.ChartExtenderBufferedFile) []*BufferedFile {
	var res []*BufferedFile
	for _, f := range files {
		f1 := new(BufferedFile)
		*f1 = BufferedFile(*f)
		res = append(res, f1)
	}
	return res
}
