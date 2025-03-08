package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	secret_common "github.com/werf/werf/v2/cmd/werf/helm/secret/common"
)

type SecretFileEditOptions struct {
	LogLevel      log.Level
	SecretWorkDir string
}

func SecretFileEdit(ctx context.Context, filePath string, opts SecretFileEditOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileEditOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file edit options: %w", err)
	}

	if err := secret_common.SecretEdit(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, false); err != nil {
		return fmt.Errorf("secret edit: %w", err)
	}

	return nil
}

func applySecretFileEditOptionsDefaults(opts SecretFileEditOptions, currentDir string) (SecretFileEditOptions, error) {
	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretFileEditOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	return opts, nil
}
