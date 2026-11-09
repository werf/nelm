package util

import (
	"archive/tar"
	"path/filepath"

	chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
)

func writeNelmFilesToTar(out *tar.Writer, c *chart.Chart, base string) error {
	for _, files := range [][]*common.File{c.RuntimeFiles, c.WerfDataFiles} {
		for _, f := range files {
			if err := writeToTar(out, filepath.Join(base, f.Name), f.Data, f.ModTime); err != nil {
				return err
			}
		}
	}

	return nil
}
