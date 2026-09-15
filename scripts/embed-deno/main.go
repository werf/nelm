// Command embed-deno downloads the pinned Deno release for one platform and compresses it into
// pkg/ts/embed/<os>/<arch>/deno.gz, which the embeddeno build tag compiles into the release binary.
// The blob is not committed, so what makes it the Deno this repository pinned is pkg/ts/denolock,
// which both the download here and the extraction at run time verify against.
package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
	"github.com/werf/nelm/pkg/ts/denolock"
)

func run() error {
	embedRoot := os.Getenv("DENO_EMBED_ROOT")
	if embedRoot == "" {
		embedRoot = filepath.Join("pkg", "ts", "embed")
	}

	platform := os.Getenv("DENO_EMBED_PLATFORM")
	if platform == "" {
		return fmt.Errorf("DENO_EMBED_PLATFORM is required: want <os>/<arch>")
	}

	goos, goarch, found := strings.Cut(platform, "/")
	if !found {
		return fmt.Errorf("parse DENO_EMBED_PLATFORM %q: want <os>/<arch>", platform)
	}

	if _, err := denolock.Get(goos, goarch); err != nil {
		return fmt.Errorf("unsupported DENO_EMBED_PLATFORM %q: %w", platform, err)
	}

	if err := embedPlatform(context.Background(), goos, goarch, embedRoot); err != nil {
		return fmt.Errorf("embed %s/%s: %w", goos, goarch, err)
	}

	return nil
}

func embedPlatform(ctx context.Context, goos, goarch, embedRoot string) error {
	platformDir := filepath.Join(embedRoot, goos, goarch)
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", platformDir, err)
	}

	downloadDir, err := os.MkdirTemp("", "embed-deno-*")
	if err != nil {
		return fmt.Errorf("create temp download directory: %w", err)
	}

	defer os.RemoveAll(downloadDir)

	binaryPath, err := ts.DownloadDenoForPlatform(ctx, goos, goarch, downloadDir)
	if err != nil {
		return fmt.Errorf("download deno: %w", err)
	}

	binaryFile, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open downloaded deno: %w", err)
	}

	defer binaryFile.Close()

	gzPath := filepath.Join(platformDir, "deno.gz")

	gzTmpFile, err := os.CreateTemp(platformDir, "deno.gz.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", platformDir, err)
	}

	gzTmpPath := gzTmpFile.Name()
	defer os.Remove(gzTmpPath)

	gzWriter := gzip.NewWriter(gzTmpFile)

	if _, err := io.Copy(gzWriter, binaryFile); err != nil {
		gzTmpFile.Close()

		return fmt.Errorf("compress deno: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		gzTmpFile.Close()

		return fmt.Errorf("close gzip writer: %w", err)
	}

	if err := gzTmpFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", gzTmpPath, err)
	}

	if err := os.Remove(gzPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", gzPath, err)
	}

	if err := os.Rename(gzTmpPath, gzPath); err != nil {
		return fmt.Errorf("rename %s to %s: %w", gzTmpPath, gzPath, err)
	}

	// Written by an older embed-deno, replaced by pkg/ts/denolock.
	if err := os.Remove(filepath.Join(platformDir, "deno.sha256")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", filepath.Join(platformDir, "deno.sha256"), err)
	}

	version, err := denolock.Version()
	if err != nil {
		return fmt.Errorf("get the pinned Deno version: %w", err)
	}

	log.Default.Info(ctx, "Embedded Deno %s for %s/%s", version, goos, goarch)

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Default.Error(context.Background(), "embed-deno: %v", err)
		os.Exit(1)
	}
}
