package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
)

var platforms = [][2]string{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

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

	if !lo.ContainsBy(platforms, func(p [2]string) bool { return p[0] == goos && p[1] == goarch }) {
		return fmt.Errorf("unsupported DENO_EMBED_PLATFORM %q", platform)
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
	sha256Path := filepath.Join(platformDir, "deno.sha256")

	gzTmpFile, err := os.CreateTemp(platformDir, "deno.gz.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", platformDir, err)
	}

	gzTmpPath := gzTmpFile.Name()
	defer os.Remove(gzTmpPath)

	hasher := sha256.New()
	gzWriter := gzip.NewWriter(gzTmpFile)

	if _, err := io.Copy(io.MultiWriter(gzWriter, hasher), binaryFile); err != nil {
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

	decompressedSha256 := hex.EncodeToString(hasher.Sum(nil))

	sha256TmpPath := sha256Path + ".tmp"
	if err := os.WriteFile(sha256TmpPath, []byte(decompressedSha256+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sha256TmpPath, err)
	}

	defer os.Remove(sha256TmpPath)

	if err := os.Remove(gzPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", gzPath, err)
	}

	if err := os.Remove(sha256Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", sha256Path, err)
	}

	if err := os.Rename(gzTmpPath, gzPath); err != nil {
		return fmt.Errorf("rename %s to %s: %w", gzTmpPath, gzPath, err)
	}

	if err := os.Rename(sha256TmpPath, sha256Path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", sha256TmpPath, sha256Path, err)
	}

	log.Default.Info(ctx, "Embedded deno for %s/%s (sha256 %s)", goos, goarch, decompressedSha256)

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Default.Error(context.Background(), "embed-deno: %v", err)
		os.Exit(1)
	}
}
