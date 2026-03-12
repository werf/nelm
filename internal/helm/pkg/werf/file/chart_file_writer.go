package file

import (
	"context"
)

type ChartFileWriterInterface interface {
	WriteChartFile(ctx context.Context, filePath string, data []byte) error
	CreateChartDir(ctx context.Context, dir string) error
}

var ChartFileWriter ChartFileWriterInterface
