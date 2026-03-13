package secrets

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/werf/nelm/internal/helm/pkg/werf/file"
	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/common-go/pkg/util"
)

const (
	DefaultSecretValuesFileName = "secret-values.yaml"
	SecretDirName               = "secret"
)

func GetDefaultSecretValuesFile(loadedChartFiles []*file.ChartExtenderBufferedFile) *file.ChartExtenderBufferedFile {
	for _, file := range loadedChartFiles {
		if file.Name == DefaultSecretValuesFileName {
			return file
		}
	}

	return nil
}

func GetSecretDirFiles(loadedChartFiles []*file.ChartExtenderBufferedFile) []*file.ChartExtenderBufferedFile {
	var res []*file.ChartExtenderBufferedFile

	for _, file := range loadedChartFiles {
		if !util.IsSubpathOfBasePath(SecretDirName, file.Name) {
			continue
		}
		res = append(res, file)
	}

	return res
}

func LoadChartSecretDirFilesData(
	secretFiles []*file.ChartExtenderBufferedFile,
	encoder *secret.YamlEncoder,
) (map[string]string, error) {
	res := make(map[string]string)

	for _, file := range secretFiles {
		if !util.IsSubpathOfBasePath(SecretDirName, file.Name) {
			continue
		}

		decodedData, err := encoder.Decrypt([]byte(strings.TrimRightFunc(string(file.Data), unicode.IsSpace)))
		if err != nil {
			return nil, fmt.Errorf("error decoding %s: %w", file.Name, err)
		}

		relPath := util.GetRelativeToBaseFilepath(SecretDirName, file.Name)
		res[filepath.ToSlash(relPath)] = string(decodedData)
	}

	return res, nil
}
