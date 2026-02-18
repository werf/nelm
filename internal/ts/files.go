package ts

import (
	"os"
	"strings"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/nelm/pkg/common"
)

func extractSourceFiles(files []*helmchart.File) map[string][]byte {
	sourceFiles := make(map[string][]byte)
	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir+"src/") {
			sourceFiles[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	return sourceFiles
}

func findEntrypointInFiles(files map[string][]byte) string {
	for _, ep := range common.ChartTSEntryPoints {
		if _, ok := files[ep]; ok {
			return ep
		}
	}

	return ""
}

func findEntrypointInDir(files []os.DirEntry) string {
	for _, f := range files {
		if f.IsDir() {
			continue
		}

		name := "src/" + f.Name()
		for _, ep := range common.ChartTSEntryPoints {
			if name == ep {
				return ep
			}
		}
	}

	return ""
}
