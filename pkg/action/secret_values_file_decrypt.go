package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/secret"
)

const DefaultSecretValuesFileDecryptOutputFilename = "decrypted-secret-values.yaml"

type SecretValuesFileDecryptOptions struct {
	LogLevel       log.Level
	OutputFilePath string
	OutputFileSave bool
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretValuesFileDecrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileDecryptOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileDecryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file decrypt options: %w", err)
	}

	var outputFilePath string
	if opts.OutputFileSave {
		outputFilePath = opts.OutputFilePath
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretValuesDecrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, valuesFilePath, outputFilePath); err != nil {
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

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretValuesFileDecryptOutputFilename)
		}
	}

	return opts, nil
}
