package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/pkg/secret"
)

const (
	DefaultSecretFileEncryptOutputFilename = "secret-file-encrypt-output.yaml"
	DefaultSecretFileEncryptLogLevel       = ErrorLogLevel
)

type SecretFileEncryptOptions struct {
	LogColorMode   string
	LogLevel       string
	OutputFilePath string
	OutputFileSave bool
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileEncrypt(ctx context.Context, filePath string, opts SecretFileEncryptOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, log.Level(opts.LogLevel))
	} else {
		log.Default.SetLevel(ctx, log.Level(DefaultSecretFileEncryptLogLevel))
	}

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

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
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

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, opts.OutputFileSave)

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultSecretFileEncryptOutputFilename)
		}
	}

	return opts, nil
}
