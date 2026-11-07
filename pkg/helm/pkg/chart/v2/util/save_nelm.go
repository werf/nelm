package util

import (
	"archive/tar"
	"path/filepath"

	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func writeWerfDataFilesToTar(out *tar.Writer, c *chart.Chart, base string) error {
	for _, f := range c.WerfDataFiles {
		if err := writeToTar(out, filepath.Join(base, f.Name), f.Data, f.ModTime); err != nil {
			return err
		}
	}

	return nil
}
