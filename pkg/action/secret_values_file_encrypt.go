package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	secret "github.com/werf/nelm/pkg/secret"
)

const DefaultSecretValuesFileEncryptOutputFilename = "encrypted-secret-values.yaml"

type SecretValuesFileEncryptOptions struct {
	LogLevel       log.Level
	OutputFilePath string
	OutputFileSave bool
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretValuesFileEncrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileEncryptOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileEncryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file encrypt options: %w", err)
	}

	var outputFilePath string
	if opts.OutputFileSave {
		outputFilePath = opts.OutputFilePath
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretValuesEncrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, valuesFilePath, outputFilePath); err != nil {
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

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretValuesFileEncryptOutputFilename)
		}
	}

	return opts, nil
}
