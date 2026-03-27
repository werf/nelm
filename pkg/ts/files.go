package ts

import (
	"strings"

	"github.com/werf/nelm/pkg/common"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
)

func extractSourceFiles(files []*chartcommon.File) map[string][]byte {
	sourceFiles := make(map[string][]byte)
	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir+"src/") {
			sourceFiles[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	return sourceFiles
}

func filterTSFiles(files []*common.BufferedFile) map[string][]byte {
	result := make(map[string][]byte)
	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir) {
			result[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	return result
}

func findEntrypointInFiles(files map[string][]byte) string {
	for _, ep := range common.ChartTSEntryPoints {
		if _, ok := files[ep]; ok {
			return ep
		}
	}

	return ""
}

func hasNodeModules(files map[string][]byte) bool {
	for name := range files {
		if strings.HasPrefix(name, "node_modules/") {
			return true
		}
	}

	return false
}
