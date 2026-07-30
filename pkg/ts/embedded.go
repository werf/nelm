//go:build embeddeno

package ts

import (
	"context"
	"strings"
)

func embeddedDenoBinary(ctx context.Context) (string, bool, error) {
	path, err := ExtractEmbeddedDeno(ctx, embeddedDeno, strings.TrimSpace(embeddedDenoSHA256))
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}
