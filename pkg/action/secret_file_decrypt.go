package action

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/lo"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/legacy/secret"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultSecretFileDecryptLogLevel = log.ErrorLevel
)

type SecretFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

func SecretFileDecrypt(ctx context.Context, filePath string, opts SecretFileDecryptOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileDecryptOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file decrypt options: %w", err)
	}

	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	if err := secret.SecretFileDecrypt(ctx, secrets_manager.Manager, opts.SecretWorkDir, filePath, opts.OutputFilePath); err != nil {
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

	return opts, nil
}
