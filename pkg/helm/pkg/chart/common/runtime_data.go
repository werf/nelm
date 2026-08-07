package common

import (
	"context"

	"github.com/werf/common-go/pkg/secrets_manager"
	nelmcommon "github.com/werf/nelm/pkg/common"
)

type RuntimeData interface {
	DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*nelmcommon.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error
	GetDecryptedSecretValues() map[string]interface{}
	GetDecryptedSecretFilesData() map[string]string
}

type DecodeAndLoadSecretsOptions struct {
	CustomSecretValueFiles     []string
	LoadFromLocalFilesystem    bool
	NoDecryptSecrets           bool
	SecretsWorkingDir          string
	WithoutDefaultSecretValues bool
}
