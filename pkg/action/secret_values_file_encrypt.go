package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/legacy/secret"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultSecretValuesFileEncryptLogLevel = log.ErrorLevel
)

type SecretValuesFileEncryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretValuesFileEncrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileEncryptOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileEncryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file encrypt options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretValuesEncrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, valuesFilePath, opts.OutputFilePath); err != nil {
		return fmt.Errorf("secret values encrypt: %w", err)
	}

	return nil
}

func applySecretValuesFileEncryptOptionsDefaults(opts SecretValuesFileEncryptOptions, currentDir string) (SecretValuesFileEncryptOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretValuesFileEncryptOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error

		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretValuesFileEncryptOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	return opts, nil
}
