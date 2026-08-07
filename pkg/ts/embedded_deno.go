package ts

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"

	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/log"
)

// extractEmbeddedDeno decompresses an embedded Deno binary into a cache
// directory keyed by expectedSHA256 and returns the path of the extracted
// binary. expectedSHA256 must be the checksum of the decompressed binary.
func extractEmbeddedDeno(ctx context.Context, compressedDeno []byte, expectedSHA256 string) (string, error) {
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if _, err := hex.DecodeString(expectedSHA256); err != nil || len(expectedSHA256) != 64 {
		return "", fmt.Errorf("unexpected embedded deno checksum format: %q", expectedSHA256)
	}

	cacheDir := helmpath.CachePath("nelm", "deno", "embedded-"+expectedSHA256[:16])
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory for embedded deno: %w", err)
	}

	denoPath := filepath.Join(cacheDir, denoBinaryName(runtime.GOOS))
	if _, err := os.Stat(denoPath); err == nil {
		log.Default.Debug(ctx, "Using cached embedded Deno binary: %s", denoPath)

		return denoPath, nil
	}

	lockFile := filepath.Join(cacheDir, "lock")

	fileLock := flock.New(lockFile)
	if err := fileLock.Lock(); err != nil {
		return "", fmt.Errorf("acquire lock on embedded deno cache: %w", err)
	}

	defer func() {
		if err := fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on embedded deno cache: %v", err)
		}
	}()

	if _, err := os.Stat(denoPath); err == nil {
		log.Default.Debug(ctx, "Using cached embedded Deno binary: %s", denoPath)

		return denoPath, nil
	}

	log.Default.Debug(ctx, "Extracting embedded Deno binary to %s", denoPath)

	if err := extractDeno(compressedDeno, expectedSHA256, denoPath); err != nil {
		return "", fmt.Errorf("extract embedded deno: %w", err)
	}

	return denoPath, nil
}

func extractDeno(compressedDeno []byte, expectedSHA256, denoPath string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(compressedDeno))
	if err != nil {
		return fmt.Errorf("init gzip reader: %w", err)
	}

	defer gzReader.Close()

	destDir := filepath.Dir(denoPath)

	tmpFile, err := os.CreateTemp(destDir, filepath.Base(denoPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", destDir, err)
	}

	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()

	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), gzReader); err != nil {
		tmpFile.Close()

		return fmt.Errorf("decompress: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA256 != expectedSHA256 {
		return fmt.Errorf("integrity check failed: expected sha256 %s, got %s", expectedSHA256, actualSHA256)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, denoPath); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, denoPath, err)
	}

	return nil
}
