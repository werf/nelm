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
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/werf/nelm/internal/helm/internal/sympath"
	"github.com/werf/nelm/internal/helm/pkg/chart"
	"github.com/werf/nelm/internal/helm/pkg/ignore"
	"github.com/werf/nelm/internal/helm/pkg/werf/file"
	"github.com/werf/nelm/internal/helm/pkg/werf/helmopts"
)

var utf8bom = []byte{0xEF, 0xBB, 0xBF}

// DirLoader loads a chart from a directory
type DirLoader string

// Load loads the chart
func (l DirLoader) Load(opts helmopts.HelmOptions) (*chart.Chart, error) {
	return LoadDir(string(l), opts)
}

// LoadDir loads from a directory.
//
// This loads charts only from directories.
func LoadDir(dir string, opts helmopts.HelmOptions) (*chart.Chart, error) {
	ctx := context.Background()

	var files []*BufferedFile
	switch opts.ChartLoadOpts.ChartType {
	case helmopts.ChartTypeChart:
		var chartTreeFiles []*file.ChartExtenderBufferedFile
		if file.ChartFileReader != nil {
			chartFiles, err := file.ChartFileReader.LoadChartDir(ctx, dir)
			if err != nil {
				return nil, fmt.Errorf("load chart dir: %w", err)
			}

			chartTreeFiles, err = LoadChartDependencies(
				ctx,
				file.ChartFileReader.LoadChartDir,
				dir,
				chartFiles,
				opts,
			)
			if err != nil {
				return nil, fmt.Errorf("load chart dependencies: %w", err)
			}
		} else {
			var chartFiles []*file.ChartExtenderBufferedFile
			if files, err := GetFilesFromLocalFilesystem(dir); err != nil {
				return nil, fmt.Errorf("load files from filesystem: %w", err)
			} else {
				chartFiles = convertBufferedFilesForChartExtender(files)
			}

			var err error
			chartTreeFiles, err = LoadChartDependencies(
				ctx,
				func(ctx context.Context, dir string) ([]*file.ChartExtenderBufferedFile, error) {
					files, err := GetFilesFromLocalFilesystem(dir)
					if err != nil {
						return nil, fmt.Errorf("load files from filesystem: %w", err)
					}

					return convertBufferedFilesForChartExtender(files), nil
				},
				dir,
				chartFiles,
				opts,
			)
			if err != nil {
				return nil, fmt.Errorf("load chart dependencies: %w", err)
			}
		}

		files = convertChartExtenderFilesToBufferedFiles(chartTreeFiles)
	case helmopts.ChartTypeSubchart:
		var err error
		files, err = GetFilesFromLocalFilesystem(dir)
		if err != nil {
			return &chart.Chart{}, err
		}
	case helmopts.ChartTypeChartStub:
		var err error
		files, err = GetFilesFromLocalFilesystem(dir)
		if err != nil {
			return &chart.Chart{}, err
		}
	case helmopts.ChartTypeBundle:
		chartFiles, err := GetFilesFromLocalFilesystem(dir)
		if err != nil {
			return nil, fmt.Errorf("load files from filesystem: %w", err)
		}

		chartTreeFiles, err := LoadChartDependencies(
			ctx,
			func(ctx context.Context, dir string) ([]*file.ChartExtenderBufferedFile, error) {
				files, err := GetFilesFromLocalFilesystem(dir)
				if err != nil {
					return nil, fmt.Errorf("load files from filesystem: %w", err)
				}

				return convertBufferedFilesForChartExtender(files), nil
			},
			dir,
			convertBufferedFilesForChartExtender(chartFiles),
			opts,
		)
		if err != nil {
			return nil, fmt.Errorf("load chart dependencies: %w", err)
		}

		files = convertChartExtenderFilesToBufferedFiles(chartTreeFiles)
	default:
		panic("unexpected type")
	}

	return LoadFiles(files, opts)
}

func GetFilesFromLocalFilesystem(dir string) ([]*BufferedFile, error) {
	topdir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	rules := ignore.Empty()
	ifile := filepath.Join(topdir, ignore.HelmIgnore)
	if _, err := os.Stat(ifile); err == nil {
		r, err := ignore.ParseFile(ifile)
		if err != nil {
			return nil, err
		}
		rules = r
	}
	rules.AddDefaults()

	files := []*BufferedFile{}
	topdir += string(filepath.Separator)

	walk := func(name string, fi os.FileInfo, err error) error {
		n := strings.TrimPrefix(name, topdir)
		if n == "" {
			// No need to process top level. Avoid bug with helmignore .* matching
			// empty names. See issue 1779.
			return nil
		}

		// Normalize to / since it will also work on Windows
		n = filepath.ToSlash(n)

		if err != nil {
			return err
		}
		if fi.IsDir() {
			// Directory-based ignore rules should involve skipping the entire
			// contents of that directory.
			if rules.Ignore(n, fi) {
				return filepath.SkipDir
			}
			return nil
		}

		// If a .helmignore file matches, skip this file.
		if rules.Ignore(n, fi) {
			return nil
		}

		// Irregular files include devices, sockets, and other uses of files that
		// are not regular files. In Go they have a file mode type bit set.
		// See https://golang.org/pkg/os/#FileMode for examples.
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("cannot load irregular file %s as it has file mode type bits set", name)
		}

		data, err := os.ReadFile(name)
		if err != nil {
			return errors.Wrapf(err, "error reading %s", n)
		}

		data = bytes.TrimPrefix(data, utf8bom)

		files = append(files, &BufferedFile{Name: n, Data: data})
		return nil
	}
	if err = sympath.Walk(topdir, walk); err != nil {
		return nil, err
	}

	return files, nil
}
