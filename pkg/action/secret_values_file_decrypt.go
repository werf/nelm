package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/secret"
)

const (
	DefaultSecretValuesFileDecryptLogLevel = ErrorLogLevel
)

type SecretValuesFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretValuesFileDecrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileDecryptOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileDecryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file decrypt options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretValuesDecrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, valuesFilePath, opts.OutputFilePath); err != nil {
		return fmt.Errorf("secret values decrypt: %w", err)
	}

	return nil
}

func applySecretValuesFileDecryptOptionsDefaults(opts SecretValuesFileDecryptOptions, currentDir string) (SecretValuesFileDecryptOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretValuesFileDecryptOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretValuesFileDecryptOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	return opts, nil
}
