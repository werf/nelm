package loader

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/logboek"
	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

type fakeChartDepsDownloader struct {
	chartPath string
	locks     []*v2chart.Lock
}

func (d *fakeChartDepsDownloader) Build(context.Context) error {
	data, err := os.ReadFile(filepath.Join(d.chartPath, "Chart.lock"))
	if err != nil {
		return err
	}

	lock := new(v2chart.Lock)
	if err := yaml.Unmarshal(data, lock); err != nil {
		return err
	}

	d.locks = append(d.locks, lock)

	for _, dep := range lock.Dependencies {
		archivePath := filepath.Join(d.chartPath, "charts", makeTestArchiveName(dep.Name, dep.Version))
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			return err
		}

		if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func (d *fakeChartDepsDownloader) Update(context.Context) error { return nil }

func (d *fakeChartDepsDownloader) UpdateRepositories(context.Context) error { return nil }

func (d *fakeChartDepsDownloader) SetChartPath(path string) { d.chartPath = path }

func makeTestArchiveName(name, version string) string {
	return name + "-" + version + ".tgz"
}

func TestLoadChartDependencies_DownloadsOnlyUnvendoredVersionOfAliasedChart(t *testing.T) {
	for _, lockOrder := range [][]string{{"1.0.0", "2.0.0"}, {"2.0.0", "1.0.0"}} {
		t.Run(lockOrder[0]+" first", func(t *testing.T) {
			localCacheDir, err := LocalCacheDir()
			require.NoError(t, err)
			serviceDir, err := ServiceDir()
			require.NoError(t, err)

			SetLocalCacheDir(t.TempDir())
			SetServiceDir(t.TempDir())

			t.Cleanup(func() {
				SetLocalCacheDir(localCacheDir)
				SetServiceDir(serviceDir)
			})

			chartLock := "dependencies:\n"
			for _, version := range lockOrder {
				chartLock += "- name: dep\n  repository: https://example.invalid\n  version: " + version + "\n"
			}

			chartLock += "digest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n"

			files := []*nelmcommon.BufferedFile{
				{Name: "Chart.yaml", Data: []byte("apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n- name: dep\n  alias: old\n  version: 1.0.0\n  repository: https://example.invalid\n- name: dep\n  alias: new\n  version: 2.0.0\n  repository: https://example.invalid\n")},
				{Name: "Chart.lock", Data: []byte(chartLock)},
				{Name: "charts/dep/Chart.yaml", Data: []byte("apiVersion: v2\nname: dep\nversion: 2.0.0\n")},
			}

			downloader := &fakeChartDepsDownloader{}
			ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))

			result, err := LoadChartDependencies(ctx, nil, t.TempDir(), files, nelmcommon.HelmOptions{
				ChartLoadOpts: nelmcommon.ChartLoadOptions{ChartDepsDownloader: downloader},
			})
			require.NoError(t, err)

			require.Len(t, downloader.locks, 1)
			require.Len(t, downloader.locks[0].Dependencies, 1)
			assert.Equal(t, "1.0.0", downloader.locks[0].Dependencies[0].Version)

			var names []string
			for _, f := range result {
				names = append(names, f.Name)
			}

			assert.ElementsMatch(t, []string{"Chart.yaml", "Chart.lock", "charts/dep/Chart.yaml", "charts/dep-1.0.0.tgz"}, names)
		})
	}
}
