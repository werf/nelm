package ts

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/util"
)

const denoVersion = "2.7.1"

// DownloadDenoForPlatform downloads the pinned Deno release for an arbitrary
// target platform into destDir and returns the path of the extracted binary.
func DownloadDenoForPlatform(ctx context.Context, goos, goarch, destDir string) (string, error) {
	link, err := getDownloadLink(goos, goarch)
	if err != nil {
		return "", fmt.Errorf("get download link: %w", err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create destination directory: %w", err)
	}

	if err := downloadDeno(ctx, destDir, link, goos); err != nil {
		return "", fmt.Errorf("download deno: %w", err)
	}

	return filepath.Join(destDir, denoBinaryName(goos)), nil
}

func downloadDeno(ctx context.Context, cacheDir, link, goos string) error {
	httpClient := util.NewRestyClient(ctx)
	httpClient.SetTimeout(15 * time.Minute)

	expectedHash, err := fetchExpectedChecksum(ctx, httpClient, link)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}

	tmpDir, err := os.MkdirTemp(cacheDir, "download-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	defer func() {
		if err = os.RemoveAll(tmpDir); err != nil {
			log.Default.Error(ctx, "failed to remove temporary directory %s: %s", tmpDir, err)
		}
	}()

	zipFile := filepath.Join(tmpDir, "deno.zip")

	log.Default.Debug(ctx, "Downloading Deno from %s to %s", link, zipFile)

	response, err := httpClient.R().SetContext(ctx).SetOutput(zipFile).Get(link)
	if err != nil {
		return fmt.Errorf("download Deno from %s: %w", link, err)
	}

	if response.IsError() {
		return fmt.Errorf("download Deno from %s: %s", link, response.Status())
	}

	if err := verifyChecksum(ctx, zipFile, expectedHash); err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}

	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return fmt.Errorf("open downloaded Deno archive: %w", err)
	}

	defer func() {
		if err = reader.Close(); err != nil {
			log.Default.Error(ctx, "close downloaded Deno archive: %s", err)
		}
	}()

	binaryName := denoBinaryName(goos)

	var binaryFound bool
	for _, file := range reader.File {
		if file.Name != binaryName {
			continue
		}

		if err := unzipBinary(ctx, tmpDir, file); err != nil {
			return fmt.Errorf("unzip binary: %w", err)
		}

		tmpBinaryPath := filepath.Join(tmpDir, filepath.Base(file.Name))
		finalPath := filepath.Join(cacheDir, filepath.Base(file.Name))

		if err := os.Rename(tmpBinaryPath, finalPath); err != nil {
			return fmt.Errorf("move Deno binary to cache: %w", err)
		}

		log.Default.Debug(ctx, "Unzipped Deno to %s", finalPath)

		binaryFound = true

		break
	}

	if !binaryFound {
		return fmt.Errorf("deno binary not found in archive")
	}

	return nil
}

func fetchExpectedChecksum(ctx context.Context, httpClient *resty.Client, archiveURL string) (string, error) {
	checksumURL := archiveURL + ".sha256sum"

	log.Default.Debug(ctx, "Fetching Deno checksum from %s", checksumURL)

	response, err := httpClient.R().SetContext(ctx).Get(checksumURL)
	if err != nil {
		return "", fmt.Errorf("download checksum from %s: %w", checksumURL, err)
	}

	if response.IsError() {
		return "", fmt.Errorf("download checksum from %s: %s", checksumURL, response.Status())
	}

	hash, found := findChecksum(response.String())
	if !found {
		return "", fmt.Errorf("unexpected checksum format from %s: %s", checksumURL, strings.TrimSpace(response.String()))
	}

	return hash, nil
}

func denoBinaryName(goos string) string {
	return lo.Ternary(goos == "windows", "deno.exe", "deno")
}

// findChecksum extracts a sha256 hex digest from a checksum file. Deno
// publishes plain "<hex>  <file>" for unix targets, but PowerShell
// Get-FileHash output ("Hash : <UPPERCASE HEX>") for windows ones, so the
// digest is located by shape rather than by field position.
func findChecksum(body string) (string, bool) {
	for _, field := range strings.Fields(body) {
		if len(field) != 64 {
			continue
		}

		if _, err := hex.DecodeString(field); err != nil {
			continue
		}

		return strings.ToLower(field), true
	}

	return "", false
}

func getDenoFolder(downloadURL string) (string, error) {
	hash := fnv.New32a()
	if _, err := hash.Write([]byte(downloadURL)); err != nil {
		return "", fmt.Errorf("calculate hash for Deno cache directory: %w", err)
	}

	hashStr := fmt.Sprintf("%x", hash.Sum32())

	suffix := downloadURL
	if len(suffix) > 15 {
		suffix = suffix[len(suffix)-15:]
	}

	dirName := hashStr + "-" + slug.Make(suffix)
	cacheDir := helmpath.CachePath("nelm", "deno", dirName)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory for Deno: %w", err)
	}

	return cacheDir, nil
}

func getDownloadLink(goos, goarch string) (string, error) {
	var target string

	switch {
	case goos == "linux" && goarch == "amd64":
		target = "x86_64-unknown-linux-gnu"
	case goos == "linux" && goarch == "arm64":
		target = "aarch64-unknown-linux-gnu"
	case goos == "darwin" && goarch == "amd64":
		target = "x86_64-apple-darwin"
	case goos == "darwin" && goarch == "arm64":
		target = "aarch64-apple-darwin"
	case goos == "windows" && goarch == "amd64":
		target = "x86_64-pc-windows-msvc"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}

	url := fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/deno-%s.zip", denoVersion, target)

	return url, nil
}

func unzipBinary(ctx context.Context, cacheDir string, file *zip.File) error {
	destPath := filepath.Join(cacheDir, filepath.Base(file.Name))

	denoFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file for Deno binary: %w", err)
	}

	defer func() {
		if err = denoFile.Close(); err != nil {
			log.Default.Error(ctx, "close file for Deno binary: %s", err)
		}
	}()

	fileReader, err := file.Open()
	if err != nil {
		return fmt.Errorf("open file %s in Deno archive: %w", file.Name, err)
	}

	defer func() {
		if err = fileReader.Close(); err != nil {
			log.Default.Error(ctx, "close file %s in Deno archive: %s", file.Name, err)
		}
	}()

	if _, err := io.Copy(denoFile, fileReader); err != nil {
		return fmt.Errorf("copy Deno binary to destination: %w", err)
	}

	if err := os.Chmod(destPath, 0o755); err != nil {
		return fmt.Errorf("chmod Deno binary: %w", err)
	}

	return nil
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
