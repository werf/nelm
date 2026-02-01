package ts

import (
	"strings"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/werf/file"
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

func filterTSFiles(files []*file.ChartExtenderBufferedFile) map[string][]byte {
	result := make(map[string][]byte)
	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir) {
			result[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	return result
}

func hasNodeModules(files map[string][]byte) bool {
	for name := range files {
		if strings.HasPrefix(name, "node_modules/") {
			return true
		}
	}

	return false
}
