package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	secret_common "github.com/werf/werf/v2/cmd/werf/helm/secret/common"
)

const DefaultSecretFileDecryptOutputFilename = "decrypted-secret.yaml"

type SecretFileDecryptOptions struct {
	LogLevel       log.Level
	OutputFilePath string
	OutputFileSave bool
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileDecrypt(ctx context.Context, filePath string, opts SecretFileDecryptOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileDecryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file decrypt options: %w", err)
	}

	var outputFilePath string
	if opts.OutputFileSave {
		outputFilePath = opts.OutputFilePath
	}

	if err := secret_common.SecretFileDecrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, outputFilePath); err != nil {
		return fmt.Errorf("secret file decrypt: %w", err)
	}

	return nil
}

func applySecretFileDecryptOptionsDefaults(opts SecretFileDecryptOptions, currentDir string) (SecretFileDecryptOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretFileDecryptOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretFileDecryptOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretFileDecryptOutputFilename)
		}
	}

	return opts, nil
}
