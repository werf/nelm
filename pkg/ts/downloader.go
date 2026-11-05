package ts

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gosimple/slug"

	"github.com/werf/nelm/v2/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/ts/nodelock"
	"github.com/werf/nelm/v2/pkg/util"
)

const downloadedNPMDirName = "npm"

// DownloadNodeForPlatform downloads the pinned Node.js release for an arbitrary target platform into
// destDir and returns the paths of the extracted Node.js binary and npm package.
func DownloadNodeForPlatform(ctx context.Context, goos, goarch, destDir string) (string, string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create destination directory: %w", err)
	}

	if err := downloadNode(ctx, destDir, goos, goarch); err != nil {
		return "", "", fmt.Errorf("download Node.js: %w", err)
	}

	return filepath.Join(destDir, nodelock.BinaryName(goos)), filepath.Join(destDir, downloadedNPMDirName), nil
}

// ensureDownloadedNode returns the cache directory holding the Node.js binary and npm package of the
// pinned release for the current platform, downloading them first if needed.
func ensureDownloadedNode(ctx context.Context) (string, error) {
	link, err := nodelock.ArchiveURL(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", fmt.Errorf("get the pinned Node.js release URL: %w", err)
	}

	cacheDir, err := getNodeFolder(link)
	if err != nil {
		return "", fmt.Errorf("get Node.js cache folder: %w", err)
	}

	// The binary is moved into place after npm, so it existing means both are there.
	nodePath := filepath.Join(cacheDir, nodelock.BinaryName(runtime.GOOS))

	if err := ensureCached(ctx, cacheDir, nodePath, func() error {
		return downloadNode(ctx, cacheDir, runtime.GOOS, runtime.GOARCH)
	}); err != nil {
		return "", fmt.Errorf("download Node.js: %w", err)
	}

	return cacheDir, nil
}

func downloadNode(ctx context.Context, destDir, goos, goarch string) error {
	pinned, err := nodelock.Get(goos, goarch)
	if err != nil {
		return fmt.Errorf("get the pinned Node.js release: %w", err)
	}

	link, err := nodelock.ArchiveURL(goos, goarch)
	if err != nil {
		return fmt.Errorf("get the pinned Node.js release URL: %w", err)
	}

	httpClient := util.NewRestyClient(ctx)
	httpClient.SetTimeout(15 * time.Minute)

	tmpDir, err := os.MkdirTemp(destDir, "download-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Default.Error(ctx, "failed to remove temporary directory %s: %s", tmpDir, err)
		}
	}()

	archivePath := filepath.Join(tmpDir, "archive")

	log.Default.Debug(ctx, "Downloading Node.js from %s to %s", link, archivePath)

	response, err := httpClient.R().SetContext(ctx).SetOutput(archivePath).Get(link)
	if err != nil {
		return fmt.Errorf("download Node.js from %s: %w", link, err)
	}

	if response.IsError() {
		return fmt.Errorf("download Node.js from %s: %s", link, response.Status())
	}

	if err := verifyChecksum(ctx, archivePath, pinned.ArchiveSHA256); err != nil {
		return fmt.Errorf("verify downloaded archive against the digest pinned for %s/%s: %w", goos, goarch, err)
	}

	tmpNodePath := filepath.Join(tmpDir, nodelock.BinaryName(goos))
	tmpNPMDir := filepath.Join(tmpDir, downloadedNPMDirName)

	if err := extractNodeArchive(archivePath, goos, tmpNodePath, tmpNPMDir, pinned); err != nil {
		return fmt.Errorf("extract downloaded archive for %s/%s: %w", goos, goarch, err)
	}

	npmDir := filepath.Join(destDir, downloadedNPMDirName)

	// Left behind if a previous download was interrupted between the two renames.
	if err := os.RemoveAll(npmDir); err != nil {
		return fmt.Errorf("remove %s: %w", npmDir, err)
	}

	if err := os.Rename(tmpNPMDir, npmDir); err != nil {
		return fmt.Errorf("move npm to %s: %w", npmDir, err)
	}

	nodePath := filepath.Join(destDir, nodelock.BinaryName(goos))
	if err := os.Rename(tmpNodePath, nodePath); err != nil {
		return fmt.Errorf("move Node.js binary to %s: %w", nodePath, err)
	}

	log.Default.Debug(ctx, "Extracted Node.js to %s", destDir)

	return nil
}

// extractNodeArchive extracts the Node.js binary and the npm package and checks them against the
// digests pinned for the archive. Redundant after the archive itself matched, but these are the
// digests the embedded blobs are checked against, so a mismatch here would mean the two ways of
// getting Node.js disagree.
func extractNodeArchive(archivePath, goos, nodePath, npmDir string, pinned *nodelock.Platform) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}

	defer archive.Close()

	info, err := archive.Stat()
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}

	var binarySHA256 string

	npmFileSHA256s := map[string]string{}

	if err := nodelock.ReadArchive(archive, info.Size(), goos, nodelock.ReadArchiveOptions{
		OnBinary: func(content io.Reader) error {
			digest, err := writeFileWithSHA256(nodePath, 0o755, content)
			binarySHA256 = digest

			return err
		},
		OnNPMFile: func(path string, mode fs.FileMode, content io.Reader) error {
			digest, err := writeNPMFile(npmDir, path, mode, content)
			npmFileSHA256s[path] = digest

			return err
		},
	}); err != nil {
		return fmt.Errorf("read archive: %w", err)
	}

	if binarySHA256 != pinned.BinarySHA256 {
		return fmt.Errorf("binary integrity check failed: expected sha256 %s, got %s", pinned.BinarySHA256, binarySHA256)
	}

	if npmSHA256 := nodelock.TreeSHA256(npmFileSHA256s); npmSHA256 != pinned.NPMSHA256 {
		return fmt.Errorf("npm integrity check failed: expected digest %s, got %s", pinned.NPMSHA256, npmSHA256)
	}

	return nil
}

func getNodeFolder(downloadURL string) (string, error) {
	hash := fnv.New32a()
	if _, err := hash.Write([]byte(downloadURL)); err != nil {
		return "", fmt.Errorf("calculate hash for Node.js cache directory: %w", err)
	}

	hashStr := fmt.Sprintf("%x", hash.Sum32())

	suffix := downloadURL
	if len(suffix) > 15 {
		suffix = suffix[len(suffix)-15:]
	}

	dirName := hashStr + "-" + slug.Make(suffix)
	cacheDir := helmpath.CachePath("nelm", "node", dirName)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory for Node.js: %w", err)
	}

	return cacheDir, nil
}

func verifyChecksum(ctx context.Context, filePath, expectedHash string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for checksum verification: %w", err)
	}

	defer func() {
		if err = file.Close(); err != nil {
			log.Default.Error(ctx, "close file for checksum verification: %s", err)
		}
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("calculate checksum: %w", err)
	}

	actualHash := fmt.Sprintf("%x", hash.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filePath, expectedHash, actualHash)
	}

	return nil
}
