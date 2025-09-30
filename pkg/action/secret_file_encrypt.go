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
	DefaultSecretFileEncryptLogLevel = log.ErrorLevel
)

type SecretFileEncryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileEncrypt(ctx context.Context, filePath string, opts SecretFileEncryptOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileEncryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file encrypt options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretFileEncrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, opts.OutputFilePath); err != nil {
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

	return opts, nil
}
