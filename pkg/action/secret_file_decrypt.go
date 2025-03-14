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

const (
	DefaultSecretFileDecryptOutputFilename = "secret-file-decrypt-output.yaml"
	DefaultSecretFileDecryptLogLevel       = log.ErrorLevel
)

type SecretFileDecryptOptions struct {
	LogColorMode   LogColorMode
	LogLevel       log.Level
	OutputFilePath string
	OutputFileSave bool
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileDecrypt(ctx context.Context, filePath string, opts SecretFileDecryptOptions) error {
	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, opts.LogLevel)
	} else {
		log.Default.SetLevel(ctx, DefaultSecretFileDecryptLogLevel)
	}

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

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretFileDecrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, outputFilePath); err != nil {
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

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, opts.OutputFileSave)

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretFileDecryptOutputFilename)
		}
	}

	return opts, nil
}
