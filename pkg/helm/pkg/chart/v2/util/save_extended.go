package util

import (
	"archive/tar"
	"compress/gzip"

	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
)

type SaveIntoTarOptions struct {
	Prefix string
}

func SaveIntoTar(out *tar.Writer, c *chart.Chart, opts SaveIntoTarOptions) error {
	return writeTarContents(out, c, opts.Prefix)
}

func SetGzipWriterMeta(zipper *gzip.Writer) {
	zipper.Header.Extra = headerBytes
	zipper.Header.Comment = "Helm"
}
