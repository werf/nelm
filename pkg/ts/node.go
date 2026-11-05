package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

// getNPMCLI returns the npm-cli.js of the pinned npm, taken in order of preference from the context,
// from nelm itself, or from the downloaded Node.js release.
func getNPMCLI(ctx context.Context) (string, error) {
	npmDir, err := getNPMDir(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(npmDir, "bin", "npm-cli.js"), nil
}

func getNPMDir(ctx context.Context) (string, error) {
	if opts := GetTSOptionsFromContext(ctx); len(opts.EmbeddedNPMCompressed) > 0 {
		pinned, err := nodelock.Get(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", fmt.Errorf("get the pinned Node.js release: %w", err)
		}

		return extractEmbeddedNPM(ctx, opts.EmbeddedNPMCompressed, pinned.NPMSHA256)
	}

	embeddedDir, embedded, err := embeddedNPMDir(ctx)
	if err != nil {
		return "", fmt.Errorf("get embedded npm: %w", err)
	}

	if embedded {
		return embeddedDir, nil
	}

	cacheDir, err := ensureDownloadedNode(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, downloadedNPMDirName), nil
}

// getNodeBinary returns, in order of preference: binaryPath, the Node.js binary the host program
// passed through the context, the one embedded into nelm, or the downloaded one.
func getNodeBinary(ctx context.Context, binaryPath string) (string, error) {
	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err != nil {
			return "", fmt.Errorf("node binary not found on path %q", binaryPath)
		}

		// Node.js and npm run in the chart directories, where a relative path would not resolve.
		absBinaryPath, err := filepath.Abs(binaryPath)
		if err != nil {
			return "", fmt.Errorf("get absolute path of %q: %w", binaryPath, err)
		}

		return absBinaryPath, nil
	}

	if opts := GetTSOptionsFromContext(ctx); len(opts.EmbeddedNodeCompressed) > 0 {
		pinned, err := nodelock.Get(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", fmt.Errorf("get the pinned Node.js release: %w", err)
		}

		return extractEmbeddedNode(ctx, opts.EmbeddedNodeCompressed, pinned.BinarySHA256)
	}

	embeddedPath, embedded, err := embeddedNodeBinary(ctx)
	if err != nil {
		return "", fmt.Errorf("get embedded Node.js binary: %w", err)
	}

	if embedded {
		return embeddedPath, nil
	}

	cacheDir, err := ensureDownloadedNode(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, nodelock.BinaryName(runtime.GOOS)), nil
}
