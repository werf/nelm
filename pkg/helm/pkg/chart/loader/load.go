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
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	nelmcommon "github.com/werf/nelm/pkg/common"
	c3 "github.com/werf/nelm/pkg/helm/intern/chart/v3"
	c3load "github.com/werf/nelm/pkg/helm/intern/chart/v3/loader"
	"github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader/archive"
	c2 "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	c2load "github.com/werf/nelm/pkg/helm/pkg/chart/v2/loader"
)

// ChartLoader loads a chart.
type ChartLoader interface {
	Load(ctx context.Context) (chart.Charter, error)
}

// Loader returns a new ChartLoader appropriate for the given chart name
func Loader(name string) (ChartLoader, error) {
	isDir, err := loader(name)
	if err != nil {
		return nil, err
	}
	if isDir {
		return DirLoader(name), nil
	}
	return FileLoader(name), nil
}

func loader(name string) (bool, error) {
	if nelmcommon.ChartFileReader == nil {
		fi, err := os.Stat(name)
		if err != nil {
			return false, err
		}

		return fi.IsDir(), nil
	}

	return nelmcommon.ChartFileReader.ChartIsDir(name)
}

// Load takes a string name, tries to resolve it to a file or directory, and then loads it.
//
// This is the preferred way to load a chart. It will discover the chart encoding
// and hand off to the appropriate chart reader.
//
// If a .helmignore file is present, the directory loader will skip loading any files
// matching it. But .helmignore is not evaluated when reading out of an archive.
func Load(ctx context.Context, name string) (chart.Charter, error) {
	l, err := Loader(name)
	if err != nil {
		return nil, err
	}

	return l.Load(ctx)
}

// DirLoader loads a chart from a directory
type DirLoader string

// Load loads the chart
func (l DirLoader) Load(ctx context.Context) (chart.Charter, error) {
	return LoadDir(ctx, string(l))
}

func LoadDir(ctx context.Context, dir string) (chart.Charter, error) {
	if nelmcommon.HasHelmOptions(ctx) {
		return loadDirWerf(ctx, dir)
	}

	return loadDirVanilla(ctx, dir)
}

func loadDirVanilla(ctx context.Context, dir string) (chart.Charter, error) {
	topdir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	name := filepath.Join(topdir, "Chart.yaml")

	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("unable to detect chart at %s: %w", name, err)
	}

	c := new(chartBase)
	if err = yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("cannot load Chart.yaml: %w", err)
	}

	switch c.APIVersion {
	case c2.APIVersionV1, c2.APIVersionV2, "":
		return c2load.Load(ctx, dir)
	case c3.APIVersionV3:
		return c3load.Load(ctx, dir)
	default:
		return nil, errors.New("unsupported chart version")
	}
}

func loadDirWerf(ctx context.Context, dir string) (chart.Charter, error) {
	helmOpts := nelmcommon.HelmOptionsFromContext(ctx)

	var chartFiles []*nelmcommon.BufferedFile
	switch helmOpts.ChartLoadOpts.ChartType {
	case nelmcommon.LegacyChartTypeChart:
		if nelmcommon.ChartFileReader != nil {
			var err error
			chartFiles, err = nelmcommon.ChartFileReader.LoadChartDir(ctx, dir)
			if err != nil {
				return nil, fmt.Errorf("load chart dir: %w", err)
			}
		} else {
			localFiles, err := getFilesFromLocalFilesystem(dir)
			if err != nil {
				return nil, fmt.Errorf("load chart dir from filesystem: %w", err)
			}

			chartFiles = localFiles
		}
	case nelmcommon.LegacyChartTypeBundle, nelmcommon.LegacyChartTypeSubchart, nelmcommon.LegacyChartTypeChartStub:
		localFiles, err := getFilesFromLocalFilesystem(dir)
		if err != nil {
			return nil, fmt.Errorf("load chart dir from filesystem: %w", err)
		}

		chartFiles = localFiles
	default:
		return nil, fmt.Errorf("unexpected chart type: %q", helmOpts.ChartLoadOpts.ChartType)
	}

	switch helmOpts.ChartLoadOpts.ChartType {
	case nelmcommon.LegacyChartTypeChart, nelmcommon.LegacyChartTypeBundle:
		var loadChartDirFunc func(ctx context.Context, dir string) ([]*nelmcommon.BufferedFile, error)
		if nelmcommon.ChartFileReader != nil {
			loadChartDirFunc = nelmcommon.ChartFileReader.LoadChartDir
		} else {
			loadChartDirFunc = func(ctx context.Context, dir string) ([]*nelmcommon.BufferedFile, error) {
				return getFilesFromLocalFilesystem(dir)
			}
		}

		var err error
		chartFiles, err = LoadChartDependencies(ctx, loadChartDirFunc, dir, chartFiles, helmOpts)
		if err != nil {
			return nil, fmt.Errorf("load chart dependencies: %w", err)
		}
	}

	files := make([]*archive.BufferedFile, 0, len(chartFiles))
	for _, f := range chartFiles {
		files = append(files, &archive.BufferedFile{Name: f.Name, Data: f.Data})
	}

	apiVersion := detectAPIVersion(chartFiles)

	switch apiVersion {
	case c2.APIVersionV1, c2.APIVersionV2, "":
		return c2load.LoadFiles(ctx, files)
	case c3.APIVersionV3:
		return c3load.LoadFiles(ctx, files)
	default:
		return nil, fmt.Errorf("unsupported chart version: %s", apiVersion)
	}
}

func detectAPIVersion(files []*nelmcommon.BufferedFile) string {
	for _, f := range files {
		if f.Name == "Chart.yaml" {
			c := new(chartBase)
			if err := yaml.Unmarshal(f.Data, c); err == nil {
				return c.APIVersion
			}
		}
	}

	return ""
}

// FileLoader loads a chart from a file
type FileLoader string

// Load loads a chart
func (l FileLoader) Load(ctx context.Context) (chart.Charter, error) {
	return LoadFile(ctx, string(l))
}

func LoadFile(ctx context.Context, name string) (chart.Charter, error) {
	if fi, err := os.Stat(name); err != nil {
		return nil, err
	} else if fi.IsDir() {
		return nil, errors.New("cannot load a directory")
	}

	raw, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer raw.Close()

	err = archive.EnsureArchive(name, raw)
	if err != nil {
		return nil, err
	}

	files, err := archive.LoadArchiveFiles(raw)
	if err != nil {
		if errors.Is(err, gzip.ErrHeader) {
			return nil, fmt.Errorf("file '%s' does not appear to be a valid chart file (details: %w)", name, err)
		}
		return nil, errors.New("unable to load chart archive")
	}

	for _, f := range files {
		if f.Name == "Chart.yaml" {
			c := new(chartBase)
			if err := yaml.Unmarshal(f.Data, c); err != nil {
				return c, fmt.Errorf("cannot load Chart.yaml: %w", err)
			}
			switch c.APIVersion {
			case c2.APIVersionV1, c2.APIVersionV2, "":
				return c2load.Load(ctx, name)
			case c3.APIVersionV3:
				return c3load.Load(ctx, name)
			default:
				return nil, errors.New("unsupported chart version")
			}
		}
	}

	return nil, errors.New("unable to detect chart version, no Chart.yaml found")
}

// LoadArchive loads from a reader containing a compressed tar archive.
func LoadArchive(ctx context.Context, in io.Reader) (chart.Charter, error) {
	// Note: This function is for use by SDK users such as Flux.

	files, err := archive.LoadArchiveFiles(in)
	if err != nil {
		if errors.Is(err, gzip.ErrHeader) {
			return nil, fmt.Errorf("stream does not appear to be a valid chart file (details: %w)", err)
		}
		return nil, fmt.Errorf("unable to load chart archive: %w", err)
	}

	for _, f := range files {
		if f.Name == "Chart.yaml" {
			c := new(chartBase)
			if err := yaml.Unmarshal(f.Data, c); err != nil {
				return c, fmt.Errorf("cannot load Chart.yaml: %w", err)
			}
			switch c.APIVersion {
			case c2.APIVersionV1, c2.APIVersionV2, "":
				return c2load.LoadFiles(ctx, files)
			case c3.APIVersionV3:
				return c3load.LoadFiles(ctx, files)
			default:
				return nil, errors.New("unsupported chart version")
			}
		}
	}

	return nil, errors.New("unable to detect chart version, no Chart.yaml found")
}

// chartBase is used to detect the API Version for the chart to run it through the
// loader for that type.
type chartBase struct {
	APIVersion string `json:"apiVersion,omitempty"`
}
