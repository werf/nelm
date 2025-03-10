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

const DefaultSecretFileEncryptOutputFilename = "encrypted-secret.yaml"

type SecretFileEncryptOptions struct {
	LogLevel       log.Level
	OutputFilePath string
	OutputFileSave bool
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileEncrypt(ctx context.Context, filePath string, opts SecretFileEncryptOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileEncryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file encrypt options: %w", err)
	}

	var outputFilePath string
	if opts.OutputFileSave {
		outputFilePath = opts.OutputFilePath
	}

	if err := secret.SecretFileEncrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, outputFilePath); err != nil {
		return fmt.Errorf("secret file encrypt: %w", err)
	}

	return nil
}

func applySecretFileEncryptOptionsDefaults(opts SecretFileEncryptOptions, currentDir string) (SecretFileEncryptOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretFileEncryptOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretFileEncryptOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretFileEncryptOutputFilename)
		}
	}

	return opts, nil
}
