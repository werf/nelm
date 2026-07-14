package secret

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/common-go/pkg/util"
	"github.com/werf/nelm/pkg/common"
)

const (
	DefaultSecretValuesFileName = "secret-values.yaml"
	SecretDirName               = "secret"
)

func GetDefaultSecretValuesFile(loadedChartFiles []*common.BufferedFile) *common.BufferedFile {
	for _, f := range loadedChartFiles {
		if f.Name == DefaultSecretValuesFileName {
			return f
		}
	}

	return nil
}

func GetSecretDirFiles(loadedChartFiles []*common.BufferedFile) []*common.BufferedFile {
	var res []*common.BufferedFile

	for _, f := range loadedChartFiles {
		if !util.IsSubpathOfBasePath(SecretDirName, f.Name) {
			continue
		}
		res = append(res, f)
	}

	return res
}

func LoadChartSecretDirFilesData(secretFiles []*common.BufferedFile, encoder *secret.YamlEncoder) (map[string]string, error) {
	res := make(map[string]string)

	for _, f := range secretFiles {
		if !util.IsSubpathOfBasePath(SecretDirName, f.Name) {
			continue
		}

		decodedData, err := encoder.Decrypt([]byte(strings.TrimRightFunc(string(f.Data), unicode.IsSpace)))
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %w", f.Name, err)
		}

		relPath := util.GetRelativeToBaseFilepath(SecretDirName, f.Name)
		res[filepath.ToSlash(relPath)] = string(decodedData)
	}

	return res, nil
}
