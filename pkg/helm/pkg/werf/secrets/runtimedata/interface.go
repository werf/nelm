package runtimedata

import (
	"context"

	"github.com/werf/nelm/pkg/helm/pkg/werf/file"
	"github.com/werf/common-go/pkg/secrets_manager"
)

type RuntimeData interface {
	DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*file.ChartExtenderBufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error
	GetEncodedSecretValues(ctx context.Context, secretsManager *secrets_manager.SecretsManager, secretsWorkingDir string, noDecryptSecrets bool) (map[string]interface{}, error)
	GetDecryptedSecretValues() map[string]interface{}
	GetDecryptedSecretFilesData() map[string]string
	GetSecretValuesToMask() []string
}

type DecodeAndLoadSecretsOptions struct {
	CustomSecretValueFiles     []string
	LoadFromLocalFilesystem    bool
	NoDecryptSecrets           bool
	SecretsWorkingDir          string
	WithoutDefaultSecretValues bool
}
