package deno

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

const version = "2.7.1"

func downloadDeno(ctx context.Context, cacheDir, link string) error {
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

	binaryName := lo.Ternary(runtime.GOOS == "windows", "deno.exe", "deno")

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

	hash, _, _ := strings.Cut(strings.TrimSpace(response.String()), " ")
	if len(hash) != 64 {
		return "", fmt.Errorf("unexpected checksum format from %s: %s", checksumURL, hash)
	}

	return hash, nil
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

	slug := strings.ToLower(strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(suffix, "-"), "-"))

	dirName := hashStr + "-" + slug
	cacheDir := helmpath.CachePath("nelm", "deno", dirName)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory for Deno: %w", err)
	}

	return cacheDir, nil
}

func getDownloadLink() (string, error) {
	var target string

	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		target = "x86_64-unknown-linux-gnu"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		target = "aarch64-unknown-linux-gnu"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		target = "x86_64-apple-darwin"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		target = "aarch64-apple-darwin"
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		target = "x86_64-pc-windows-msvc"
	case runtime.GOOS == "windows" && runtime.GOARCH == "arm64":
		target = "aarch64-pc-windows-msvc"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	url := fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/deno-%s.zip", version, target)

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

	limitReader := io.LimitReader(fileReader, 200*1024*1024)
	if _, err := io.Copy(denoFile, limitReader); err != nil {
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
