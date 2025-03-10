package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/secret"
)

type SecretKeyRotateOptions struct {
	ChartDirPath      string
	LogLevel          log.Level
	NewSecretKey      string
	OldSecretKey      string
	SecretValuesPaths []string
	SecretWorkDir     string
}

func SecretKeyRotate(ctx context.Context, opts SecretKeyRotateOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

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

	return opts, nil
}
