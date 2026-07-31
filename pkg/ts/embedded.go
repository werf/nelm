//go:build embeddeno

package ts

import (
	"context"
)

func embeddedDenoBinary(ctx context.Context) (string, bool, error) {
	path, err := ExtractEmbeddedDeno(ctx, embeddedDeno, embeddedDenoSHA256)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}
