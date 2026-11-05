package ts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"

	"github.com/werf/nelm/v2/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

// extractEmbeddedNPM unpacks a gzipped tar of the npm package into a cache directory keyed by
// expectedSHA256, the nodelock.TreeSHA256 of the package, and returns the path of the package.
func extractEmbeddedNPM(ctx context.Context, compressedNPM []byte, expectedSHA256 string) (string, error) {
	cacheDir := helmpath.CachePath("nelm", "node", "embedded-npm-"+expectedSHA256[:16])
	npmDir := filepath.Join(cacheDir, "npm")

	if err := ensureCached(ctx, cacheDir, npmDir, func() error {
		log.Default.Debug(ctx, "Extracting embedded npm to %s", npmDir)

		return extractNPMTarGz(compressedNPM, expectedSHA256, npmDir)
	}); err != nil {
		return "", fmt.Errorf("extract embedded npm: %w", err)
	}

	return npmDir, nil
}

// extractEmbeddedNode decompresses a gzipped Node.js binary into a cache directory keyed by
// expectedSHA256, the checksum of the decompressed binary, and returns the path of the binary.
func extractEmbeddedNode(ctx context.Context, compressedNode []byte, expectedSHA256 string) (string, error) {
	cacheDir := helmpath.CachePath("nelm", "node", "embedded-"+expectedSHA256[:16])
	nodePath := filepath.Join(cacheDir, nodelock.BinaryName(runtime.GOOS))

	if err := ensureCached(ctx, cacheDir, nodePath, func() error {
		log.Default.Debug(ctx, "Extracting embedded Node.js binary to %s", nodePath)

		return extractGzippedBinary(compressedNode, expectedSHA256, nodePath)
	}); err != nil {
		return "", fmt.Errorf("extract embedded Node.js binary: %w", err)
	}

	return nodePath, nil
}

func extractNPMTarGz(compressed []byte, expectedSHA256, destDir string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("init gzip reader: %w", err)
	}

	defer gzReader.Close()

	tmpDir, err := os.MkdirTemp(filepath.Dir(destDir), "extract-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	defer os.RemoveAll(tmpDir)

	tarReader := tar.NewReader(gzReader)
	fileSHA256s := map[string]string{}

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		if header.Typeflag == tar.TypeDir {
			continue
		}

		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unexpected entry %q of type %q", header.Name, header.Typeflag)
		}

		digest, err := writeNPMFile(tmpDir, header.Name, header.FileInfo().Mode(), tarReader)
		if err != nil {
			return err
		}

		fileSHA256s[header.Name] = digest
	}

	if actualSHA256 := nodelock.TreeSHA256(fileSHA256s); actualSHA256 != expectedSHA256 {
		return fmt.Errorf("integrity check failed: expected npm digest %s, got %s", expectedSHA256, actualSHA256)
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpDir, destDir, err)
	}

	return nil
}

func extractGzippedBinary(compressed []byte, expectedSHA256, destPath string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("init gzip reader: %w", err)
	}

	defer gzReader.Close()

	tmpDir, err := os.MkdirTemp(filepath.Dir(destPath), "extract-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, filepath.Base(destPath))

	actualSHA256, err := writeFileWithSHA256(tmpPath, 0o755, gzReader)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}

	if actualSHA256 != expectedSHA256 {
		return fmt.Errorf("integrity check failed: expected sha256 %s, got %s", expectedSHA256, actualSHA256)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, destPath, err)
	}

	return nil
}

// writeNPMFile writes a file of the npm package under npmDir, given its slash-separated path relative
// to the package root, and returns its sha256.
func writeNPMFile(npmDir, path string, mode fs.FileMode, content io.Reader) (string, error) {
	// Localize also rejects what is a separator or a device name only on Windows. A newline would
	// make the TreeSHA256 input ambiguous.
	localPath, err := filepath.Localize(path)
	if err != nil || strings.Contains(path, "\n") {
		return "", fmt.Errorf("unsafe path %q in npm package", path)
	}

	destPath := filepath.Join(npmDir, localPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create directory for %s: %w", path, err)
	}

	perm := fs.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}

	digest, err := writeFileWithSHA256(destPath, perm, content)
	if err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return digest, nil
}

// ensureCached calls create under a lock on cacheDir unless path already exists. create must make
// path appear atomically, so that its existence means it is complete.
func ensureCached(ctx context.Context, cacheDir, path string, create func() error) error {
	if _, err := os.Stat(path); err == nil {
		log.Default.Debug(ctx, "Using cached %s", path)

		return nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache directory %s: %w", cacheDir, err)
	}

	fileLock := flock.New(filepath.Join(cacheDir, "lock"))
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on cache directory %s: %w", cacheDir, err)
	}

	defer func() {
		if err := fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on cache directory %s: %v", cacheDir, err)
		}
	}()

	if _, err := os.Stat(path); err == nil {
		log.Default.Debug(ctx, "Using cached %s", path)

		return nil
	}

	return create()
}

func writeFileWithSHA256(path string, perm fs.FileMode, content io.Reader) (string, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", path, err)
	}

	hasher := sha256.New()

	if _, err := io.Copy(io.MultiWriter(file, hasher), content); err != nil {
		file.Close()

		return "", fmt.Errorf("write %s: %w", path, err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", path, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
