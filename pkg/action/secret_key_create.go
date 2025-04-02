package action

import (
	"context"
	"fmt"
	"os"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/internal/log"
)

const (
	DefaultSecretKeyCreateLogLevel = ErrorLogLevel
)

type SecretKeyCreateOptions struct {
	LogColorMode  string
	LogLevel      string
	OutputNoPrint bool
	TempDirPath   string
}

func SecretKeyCreate(ctx context.Context, opts SecretKeyCreateOptions) (string, error) {
	actionLock.Lock()
	defer actionLock.Unlock()

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, log.Level(opts.LogLevel))
	} else {
		log.Default.SetLevel(ctx, log.Level(DefaultSecretKeyCreateLogLevel))
	}

	opts, err := applySecretKeyCreateOptionsDefaults(opts)
	if err != nil {
		return "", fmt.Errorf("build secret key create options: %w", err)
	}

	var result string
	if !opts.OutputNoPrint {
		if keyByte, err := secrets_manager.GenerateSecretKey(); err != nil {
			return "", fmt.Errorf("generate secret key: %w", err)
		} else {
			result = string(keyByte)
		}

		fmt.Println(result)
	}

	return result, nil
}

func applySecretKeyCreateOptionsDefaults(opts SecretKeyCreateOptions) (SecretKeyCreateOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return SecretKeyCreateOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}
