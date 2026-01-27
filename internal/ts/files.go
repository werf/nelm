package ts

import (
	"fmt"
	"os"
	"path/filepath"
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

func findEntrypointInDir(tsDir string) (string, error) {
	for _, ep := range common.ChartTSEntryPoints {
		epPath := filepath.Join(tsDir, ep)

		_, err := os.Stat(epPath)
		if err == nil {
			return ep, nil
		}

		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", epPath, err)
		}
	}

	return "", nil
}

func findEntrypointInFiles(files map[string][]byte) string {
	for _, ep := range common.ChartTSEntryPoints {
		if _, ok := files[ep]; ok {
			return ep
		}
	}

	return ""
}
