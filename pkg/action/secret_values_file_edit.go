package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/secret"
)

const (
	DefaultSecretValuesFileEditLogLevel = log.ErrorLevel
)

type SecretValuesFileEditOptions struct {
	LogColorMode  LogColorMode
	LogLevel      log.Level
	SecretKey     string
	SecretWorkDir string
	TempDirPath   string
}

func SecretValuesFileEdit(ctx context.Context, valuesFilePath string, opts SecretValuesFileEditOptions) error {
	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, opts.LogLevel)
	} else {
		log.Default.SetLevel(ctx, DefaultSecretValuesFileEditLogLevel)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretValuesFileEditOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret values file edit options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if err := secret.SecretEdit(ctx, secrets_manager.Manager, opts.SecretWorkDir, opts.TempDirPath, valuesFilePath, true); err != nil {
		return fmt.Errorf("secret edit: %w", err)
	}

	return nil
}

func applySecretValuesFileEditOptionsDefaults(opts SecretValuesFileEditOptions, currentDir string) (SecretValuesFileEditOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretValuesFileEditOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretValuesFileEditOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}
