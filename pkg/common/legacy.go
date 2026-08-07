package common

import "context"

const (
	LegacyChartTypeChart     LegacyChartType = ""
	LegacyChartTypeBundle    LegacyChartType = "bundle"
	LegacyChartTypeSubchart  LegacyChartType = "subchart"
	LegacyChartTypeChartStub LegacyChartType = "chartstub"
)

var (
	ChartFileReader ChartFileReaderer

	ChartFileWriter ChartFileWriterer

	LegacyCoalesceTablesFunc func(dst, src map[string]interface{}) map[string]interface{}
)

type LegacyChartType string

type ChartDepsDownloader interface {
	Build(ctx context.Context) error
	Update(ctx context.Context) error
	UpdateRepositories(ctx context.Context) error
	SetChartPath(path string)
}

type ChartFileReaderer interface {
	LocateChart(ctx context.Context, name string) (string, error)
	ReadChartFile(ctx context.Context, filePath string) ([]byte, error)
	ChartFileExists(ctx context.Context, filePath string) (bool, error)
	LoadChartDir(ctx context.Context, dir string) ([]*BufferedFile, error)
	ChartIsDir(relPath string) (bool, error)
}

type ChartFileWriterer interface {
	WriteChartFile(ctx context.Context, filePath string, data []byte) error
	CreateChartDir(ctx context.Context, dir string) error
}

type BufferedFile struct {
	Data []byte
	Name string
}
