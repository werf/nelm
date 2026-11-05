//go:build !embednode

package ts

import "context"

func embeddedNPMDir(_ context.Context) (string, bool, error) {
	return "", false, nil
}

func embeddedNodeBinary(_ context.Context) (string, bool, error) {
	return "", false, nil
}
