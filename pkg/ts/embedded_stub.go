//go:build !embeddeno

package ts

import "context"

func embeddedDenoBinary(_ context.Context) (string, bool, error) {
	return "", false, nil
}
