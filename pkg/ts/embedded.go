//go:build embeddeno

package ts

import (
	"context"
	"fmt"
	"runtime"

	"github.com/werf/nelm/pkg/ts/denolock"
)

func embeddedDenoBinary(ctx context.Context) (string, bool, error) {
	pinned, err := denolock.Get(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", false, fmt.Errorf("get the pinned Deno release: %w", err)
	}

	path, err := extractEmbeddedDeno(ctx, embeddedDeno, pinned.BinarySHA256)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}
