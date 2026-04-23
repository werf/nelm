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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secrets_manager"
	nelmcommon "github.com/werf/nelm/pkg/common"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader/archive"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	legacysecret "github.com/werf/nelm/pkg/legacy/secret"
)

// ChartLoader loads a chart.
type ChartLoader interface {
	Load(ctx context.Context) (*chart.Chart, error)
}

// Loader returns a new ChartLoader appropriate for the given chart name
func Loader(name string) (ChartLoader, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return DirLoader(name), nil
	}
	return FileLoader(name), nil
}

// Load takes a string name, tries to resolve it to a file or directory, and then loads it.
//
// This is the preferred way to load a chart. It will discover the chart encoding
// and hand off to the appropriate chart reader.
//
// If a .helmignore file is present, the directory loader will skip loading any files
// matching it. But .helmignore is not evaluated when reading out of an archive.
func Load(ctx context.Context, name string) (*chart.Chart, error) {
	l, err := Loader(name)
	if err != nil {
		return nil, err
	}

	return l.Load(ctx)
}

// LoadFiles loads from in-memory files.
func LoadFiles(ctx context.Context, files []*archive.BufferedFile) (*chart.Chart, error) {
	helmOpts := nelmcommon.HelmOptionsFromContext(ctx)
	applyWerfExtensions := nelmcommon.HasHelmOptions(ctx)

	c := new(chart.Chart)
	subcharts := make(map[string][]*archive.BufferedFile)

	if applyWerfExtensions {
		c.SecretsRuntimeData = legacysecret.NewSecretsRuntimeData()
	}

	// do not rely on assumed ordering of files in the chart and crash
	// if Chart.yaml was not coming early enough to initialize metadata
	for _, f := range files {
		c.Raw = append(c.Raw, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
		if f.Name == "Chart.yaml" {
			if c.Metadata == nil {
				c.Metadata = new(chart.Metadata)
			}
			if err := yaml.Unmarshal(f.Data, c.Metadata); err != nil {
				return c, fmt.Errorf("cannot load Chart.yaml: %w", err)
			}
			// NOTE(bacongobbler): while the chart specification says that APIVersion must be set,
			// Helm 2 accepted charts that did not provide an APIVersion in their chart metadata.
			// Because of that, if APIVersion is unset, we should assume we're loading a v1 chart.
			if c.Metadata.APIVersion == "" {
				c.Metadata.APIVersion = chart.APIVersionV1
			}
			c.ModTime = f.ModTime
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
				return c, fmt.Errorf("cannot load Chart.lock: %w", err)
			}
		case f.Name == "values.yaml":
			values, err := LoadValues(bytes.NewReader(f.Data))
			if err != nil {
				return c, fmt.Errorf("cannot load values.yaml: %w", err)
			}
			c.Values = values
		case f.Name == "values.schema.json":
			c.Schema = f.Data
			c.SchemaModTime = f.ModTime

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
				return c, fmt.Errorf("cannot load requirements.yaml: %w", err)
			}
			if c.Metadata.APIVersion == chart.APIVersionV1 {
				c.Files = append(c.Files, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
			}
		// Deprecated: requirements.lock is deprecated use Chart.lock.
		case f.Name == "requirements.lock":
			c.Lock = new(chart.Lock)
			if err := yaml.Unmarshal(f.Data, &c.Lock); err != nil {
				return c, fmt.Errorf("cannot load requirements.lock: %w", err)
			}
			if c.Metadata == nil {
				c.Metadata = new(chart.Metadata)
			}
			if c.Metadata.APIVersion != chart.APIVersionV1 {
				log.Printf("Warning: Dependency locking is handled in Chart.lock since apiVersion \"v2\". We recommend migrating to Chart.lock.")
			}
			if c.Metadata.APIVersion == chart.APIVersionV1 {
				c.Files = append(c.Files, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
			}

		case strings.HasPrefix(f.Name, "templates/"):
			c.Templates = append(c.Templates, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
		case strings.HasPrefix(f.Name, "charts/"):
			if filepath.Ext(f.Name) == ".prov" {
				c.Files = append(c.Files, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
				continue
			}

			fname := strings.TrimPrefix(f.Name, "charts/")
			cname := strings.SplitN(fname, "/", 2)[0]
			subcharts[cname] = append(subcharts[cname], &archive.BufferedFile{Name: fname, ModTime: f.ModTime, Data: f.Data})
		case applyWerfExtensions && strings.HasPrefix(f.Name, "ts/") && !strings.HasPrefix(f.Name, "ts/node_modules/"):
			c.RuntimeFiles = append(c.RuntimeFiles, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
		default:
			c.Files = append(c.Files, &chartcommon.File{Name: f.Name, ModTime: f.ModTime, Data: f.Data})
		}
	}

	if applyWerfExtensions {
		switch helmOpts.ChartLoadOpts.ChartType {
		case nelmcommon.LegacyChartTypeBundle:
			c.ExtraValues = helmOpts.ChartLoadOpts.ExtraValues

			if !helmOpts.ChartLoadOpts.NoSecrets {
				if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
					ctx,
					convertBufferedFiles(files),
					secrets_manager.Manager,
					chartcommon.DecodeAndLoadSecretsOptions{
						CustomSecretValueFiles:     helmOpts.ChartLoadOpts.SecretValuesFiles,
						LoadFromLocalFilesystem:    true,
						NoDecryptSecrets:           helmOpts.ChartLoadOpts.SecretKeyIgnore,
						SecretsWorkingDir:          helmOpts.ChartLoadOpts.SecretWorkDir,
						WithoutDefaultSecretValues: helmOpts.ChartLoadOpts.DefaultSecretValuesDisable,
					},
				); err != nil {
					return nil, fmt.Errorf("error decoding secrets: %w", err)
				}
			}

			if helmOpts.ChartLoadOpts.DefaultValuesDisable {
				c.Values = nil
			}
		case nelmcommon.LegacyChartTypeChart:
			c.ExtraValues = helmOpts.ChartLoadOpts.ExtraValues

			if !helmOpts.ChartLoadOpts.NoSecrets {
				if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
					ctx,
					convertBufferedFiles(files),
					secrets_manager.Manager,
					chartcommon.DecodeAndLoadSecretsOptions{
						CustomSecretValueFiles:     helmOpts.ChartLoadOpts.SecretValuesFiles,
						LoadFromLocalFilesystem:    nelmcommon.ChartFileReader == nil,
						NoDecryptSecrets:           helmOpts.ChartLoadOpts.SecretKeyIgnore,
						SecretsWorkingDir:          helmOpts.ChartLoadOpts.SecretWorkDir,
						WithoutDefaultSecretValues: helmOpts.ChartLoadOpts.DefaultSecretValuesDisable,
					},
				); err != nil {
					return nil, fmt.Errorf("error decoding secrets: %w", err)
				}
			}

			c.Metadata = autosetChartMetadata(
				c.Metadata,
				autosetChartMetadataOptions{
					DefaultAPIVersion:  helmOpts.ChartLoadOpts.DefaultChartAPIVersion,
					DefaultName:        helmOpts.ChartLoadOpts.DefaultChartName,
					DefaultVersion:     helmOpts.ChartLoadOpts.DefaultChartVersion,
					OverrideAppVersion: helmOpts.ChartLoadOpts.ChartAppVersion,
				},
			)

			c.Templates = append(c.Templates, &chartcommon.File{Name: "templates/_werf_helpers.tpl"})

			if helmOpts.ChartLoadOpts.DefaultValuesDisable {
				c.Values = nil
			}
		case nelmcommon.LegacyChartTypeSubchart:
			if !helmOpts.ChartLoadOpts.NoSecrets {
				if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
					ctx,
					convertBufferedFiles(files),
					secrets_manager.Manager,
					chartcommon.DecodeAndLoadSecretsOptions{
						LoadFromLocalFilesystem:    nelmcommon.ChartFileReader == nil,
						NoDecryptSecrets:           helmOpts.ChartLoadOpts.SecretKeyIgnore,
						SecretsWorkingDir:          helmOpts.ChartLoadOpts.SecretWorkDir,
						WithoutDefaultSecretValues: helmOpts.ChartLoadOpts.DefaultSecretValuesDisable,
					},
				); err != nil {
					return nil, fmt.Errorf("error decoding secrets: %w", err)
				}
			}
		case nelmcommon.LegacyChartTypeChartStub:
			if !helmOpts.ChartLoadOpts.NoSecrets {
				if err := c.SecretsRuntimeData.DecodeAndLoadSecrets(
					ctx,
					convertBufferedFiles(files),
					secrets_manager.Manager,
					chartcommon.DecodeAndLoadSecretsOptions{
						LoadFromLocalFilesystem:    true,
						NoDecryptSecrets:           helmOpts.ChartLoadOpts.SecretKeyIgnore,
						SecretsWorkingDir:          helmOpts.ChartLoadOpts.SecretWorkDir,
						WithoutDefaultSecretValues: helmOpts.ChartLoadOpts.DefaultSecretValuesDisable,
					},
				); err != nil {
					return nil, fmt.Errorf("error decoding secrets: %w", err)
				}
			}

			c.Metadata = autosetChartMetadata(
				c.Metadata,
				autosetChartMetadataOptions{
					DefaultAPIVersion: chart.APIVersionV2,
					DefaultName:       "stubchartname",
					DefaultVersion:    "1.0.0",
				},
			)

			c.Templates = append(c.Templates, &chartcommon.File{Name: "templates/_werf_helpers.tpl"})
		default:
			panic("unexpected type")
		}
	}

	if c.Metadata == nil {
		return c, errors.New("Chart.yaml file is missing") //nolint:staticcheck
	}

	if err := c.Validate(); err != nil {
		return c, err
	}

	helmOpts.ChartLoadOpts.ChartType = nelmcommon.LegacyChartTypeSubchart
	ctx = nelmcommon.ContextWithHelmOptions(ctx, helmOpts)

	for n, files := range subcharts {
		var sc *chart.Chart
		var err error
		switch {
		case strings.IndexAny(n, "_.") == 0:
			continue
		case filepath.Ext(n) == ".tgz":
			file := files[0]
			if file.Name != n {
				return c, fmt.Errorf("error unpacking subchart tar in %s: expected %s, got %s", c.Name(), n, file.Name)
			}
			sc, err = LoadArchive(ctx, bytes.NewBuffer(file.Data))
		default:
			buff := make([]*archive.BufferedFile, 0, len(files))
			for _, f := range files {
				parts := strings.SplitN(f.Name, "/", 2)
				if len(parts) < 2 {
					continue
				}
				f.Name = parts[1]
				buff = append(buff, f)
			}
			sc, err = LoadFiles(ctx, buff)
		}

		if err != nil {
			return c, fmt.Errorf("error unpacking subchart %s in %s: %w", n, c.Name(), err)
		}
		c.AddDependency(sc)
	}

	return c, nil
}

func convertBufferedFiles(files []*archive.BufferedFile) []*nelmcommon.BufferedFile {
	var res []*nelmcommon.BufferedFile
	for _, f := range files {
		res = append(res, &nelmcommon.BufferedFile{Name: f.Name, Data: f.Data})
	}

	return res
}

// LoadValues loads values from a reader.
//
// The reader is expected to contain one or more YAML documents, the values of which are merged.
// And the values can be either a chart's default values or user-supplied values.
func LoadValues(data io.Reader) (map[string]interface{}, error) {
	values := map[string]interface{}{}
	reader := utilyaml.NewYAMLReader(bufio.NewReader(data))
	for {
		currentMap := map[string]interface{}{}
		raw, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("error reading yaml document: %w", err)
		}
		if err := yaml.Unmarshal(raw, &currentMap); err != nil {
			return nil, fmt.Errorf("cannot unmarshal yaml document: %w", err)
		}
		values = MergeMaps(values, currentMap)
	}
	return values, nil
}

// MergeMaps merges two maps. If a key exists in both maps, the value from b will be used.
// If the value is a map, the maps will be merged recursively.
func MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	maps.Copy(out, a)
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = MergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
