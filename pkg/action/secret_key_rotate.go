package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/pkg/secret"
)

const (
	DefaultSecretKeyRotateLogLevel = InfoLogLevel
)

type SecretKeyRotateOptions struct {
	ChartDirPath      string
	LogColorMode      string
	LogLevel          string
	NewSecretKey      string
	OldSecretKey      string
	SecretValuesPaths []string
	SecretWorkDir     string
	TempDirPath       string
}

func SecretKeyRotate(ctx context.Context, opts SecretKeyRotateOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, log.Level(opts.LogLevel))
	} else {
		log.Default.SetLevel(ctx, log.Level(DefaultSecretKeyRotateLogLevel))
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretKeyRotateOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret key rotate options: %w", err)
	}

	if opts.OldSecretKey != "" {
		os.Setenv("WERF_OLD_SECRET_KEY", opts.OldSecretKey)
	}

	if opts.NewSecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.NewSecretKey)
	}

	if err := secret.RotateSecretKey(ctx, opts.ChartDirPath, opts.SecretWorkDir, opts.SecretValuesPaths...); err != nil {
		return fmt.Errorf("rotate secret key: %w", err)
	}

	return nil
}

func applySecretKeyRotateOptionsDefaults(opts SecretKeyRotateOptions, currentDir string) (SecretKeyRotateOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretKeyRotateOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretKeyRotateOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}
