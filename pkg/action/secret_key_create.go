package action

import (
	"context"
	"fmt"

	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/log"
)

type SecretKeyCreateOptions struct {
	OutputNoPrint bool
}

func SecretKeyCreate(ctx context.Context, opts SecretKeyCreateOptions) (string, error) {
	log.Default.SetLevel(ctx, log.SilentLevel)

	opts, err := applySecretKeyCreateOptionsDefaults(opts)
	if err != nil {
		return "", fmt.Errorf("build secret key create options: %w", err)
	}

	loader.NoChartLockWarning = ""

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
	return opts, nil
}
