//go:build embednode

package ts

import (
	"context"
	"fmt"
	"runtime"

	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

func embeddedNPMDir(ctx context.Context) (string, bool, error) {
	pinned, err := nodelock.Get(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", false, fmt.Errorf("get the pinned Node.js release: %w", err)
	}

	path, err := extractEmbeddedNPM(ctx, embeddedNPM, pinned.NPMSHA256)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}

// There are blobs only for the platforms nodelock pins, so a tagged build for any other one fails
// with "undefined: embeddedNode".
func embeddedNodeBinary(ctx context.Context) (string, bool, error) {
	pinned, err := nodelock.Get(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", false, fmt.Errorf("get the pinned Node.js release: %w", err)
	}

	path, err := extractEmbeddedNode(ctx, embeddedNode, pinned.BinarySHA256)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}
