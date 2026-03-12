package file

import internal "github.com/werf/nelm/internal/helm/pkg/werf/file"

type ChartExtenderBufferedFile = internal.ChartExtenderBufferedFile
type ChartFileReaderInterface = internal.ChartFileReaderInterface

func SetChartFileReader(v ChartFileReaderInterface) { internal.ChartFileReader = v }
func GetChartFileReader() ChartFileReaderInterface  { return internal.ChartFileReader }
