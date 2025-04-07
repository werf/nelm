package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/secret"
)

const (
	DefaultSecretFileEditLogLevel = ErrorLogLevel
)

type SecretFileEditOptions struct {
	LogColorMode  string
	SecretKey     string
	SecretWorkDir string
	TempDirPath   string
}

func SecretFileEdit(ctx context.Context, filePath string, opts SecretFileEditOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretFileEditOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret file edit options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretEdit(ctx, secrets_manager.Manager, opts.SecretWorkDir, opts.TempDirPath, filePath, false); err != nil {
		return fmt.Errorf("secret edit: %w", err)
	}

	return nil
}

func applySecretFileEditOptionsDefaults(opts SecretFileEditOptions, currentDir string) (SecretFileEditOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretFileEditOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretFileEditOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}
