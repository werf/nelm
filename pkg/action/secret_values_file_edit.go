package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	secret_common "github.com/werf/werf/v2/cmd/werf/helm/secret/common"
)

type SecretValuesFileEditOptions struct {
	LogLevel      log.Level
	SecretWorkDir string
}

func SecretValuesFileEdit(ctx context.Context, valuesFilePath string, opts SecretValuesFileEditOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileEditOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file edit options: %w", err)
	}

	if err := secret_common.SecretEdit(ctx, secrets_manager.Manager, opts.SecretWorkDir, valuesFilePath, true); err != nil {
		return fmt.Errorf("secret edit: %w", err)
	}

	return nil
}

func applySecretValuesFileEditOptionsDefaults(opts SecretValuesFileEditOptions, currentDir string) (SecretValuesFileEditOptions, error) {
	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretValuesFileEditOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	return opts, nil
}
