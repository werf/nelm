package file

import (
	"context"
)

type ChartFileReaderInterface interface {
	LocateChart(ctx context.Context, name string) (string, error)
	ReadChartFile(ctx context.Context, filePath string) ([]byte, error)
	LoadChartDir(ctx context.Context, dir string) ([]*ChartExtenderBufferedFile, error)
	ChartIsDir(relPath string) (bool, error)
}

// TODO(werf): keep it global, but separate package? Make non-giterminism default implementation
var ChartFileReader ChartFileReaderInterface
