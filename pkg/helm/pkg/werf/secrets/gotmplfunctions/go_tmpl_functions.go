package gotmplfunctions

import (
	"fmt"
	"path"
	"strings"
	"text/template"

	"github.com/werf/nelm/pkg/helm/pkg/werf/secrets/runtimedata"
)

func SetupWerfSecretFile(secretsRuntimeData runtimedata.RuntimeData, funcMap template.FuncMap) {
	funcMap["werf_secret_file"] = func(secretRelativePath string) (string, error) {
		if path.IsAbs(secretRelativePath) {
			return "", fmt.Errorf("expected relative secret file path, given path %v", secretRelativePath)
		}

		decodedData, ok := secretsRuntimeData.GetDecryptedSecretFilesData()[secretRelativePath]

		if !ok {
			var secretFiles []string
			for key := range secretsRuntimeData.GetDecryptedSecretFilesData() {
				secretFiles = append(secretFiles, key)
			}

			return "", fmt.Errorf("secret file %q not found, you may use one of the following: %q", secretRelativePath, strings.Join(secretFiles, "', '"))
		}

		return decodedData, nil
	}
}
