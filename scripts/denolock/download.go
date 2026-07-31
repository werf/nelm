package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/werf/nelm/pkg/ts/denolock"
)

const (
	downloadAttempts = 3
	retryDelay       = 3 * time.Second
	sha256HexLen     = 64
)

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	var lastErr error

	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		body, retriable, err := getOnce(ctx, client, url)
		if err == nil {
			return body, nil
		}

		lastErr = err

		if !retriable || ctx.Err() != nil {
			break
		}

		if attempt < downloadAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	return nil, lastErr
}

func fetchUpstreamChecksum(ctx context.Context, client *http.Client, archiveURL string) (string, error) {
	checksumURL := archiveURL + denolock.ChecksumURLSuffix

	body, err := download(ctx, client, checksumURL)
	if err != nil {
		return "", err
	}

	digest, found := findChecksum(string(body))
	if !found {
		return "", fmt.Errorf("no sha256 digest in %s: %s", checksumURL, strings.TrimSpace(string(body)))
	}

	return digest, nil
}

// findChecksum extracts a sha256 hex digest from a checksum file. Deno publishes plain
// "<hex>  <file>" for unix targets, but PowerShell Get-FileHash output ("Hash : <UPPERCASE HEX>") for
// windows ones, so the digest is located by shape rather than by field position.
func findChecksum(body string) (string, bool) {
	for _, field := range strings.Fields(body) {
		if len(field) != sha256HexLen {
			continue
		}

		if _, err := hex.DecodeString(field); err != nil {
			continue
		}

		return strings.ToLower(field), true
	}

	return "", false
}

func extractDenoBinary(archive []byte, goos string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	binaryName := "deno"
	if goos == "windows" {
		binaryName = "deno.exe"
	}

	for _, file := range reader.File {
		if file.Name != binaryName {
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in archive: %w", binaryName, err)
		}

		defer fileReader.Close()

		binary, err := io.ReadAll(fileReader)
		if err != nil {
			return nil, fmt.Errorf("read %s from archive: %w", binaryName, err)
		}

		return binary, nil
	}

	return nil, fmt.Errorf("no %s in archive", binaryName)
}

func latestVersion(ctx context.Context, client *http.Client) (string, error) {
	body, err := download(ctx, client, latestReleaseURL)
	if err != nil {
		return "", err
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("decode response of %s: %w", latestReleaseURL, err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("%s reports no tag name", latestReleaseURL)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

func getOnce(ctx context.Context, client *http.Client, url string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create request for %s: %w", url, err)
	}

	// Authenticating lifts the GitHub API rate limit, which is easy to hit on shared CI runners.
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && strings.HasPrefix(url, "https://api.github.com/") {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, true, fmt.Errorf("get %s: %w", url, err)
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read response of %s: %w", url, err)
	}

	if response.StatusCode != http.StatusOK {
		retriable := response.StatusCode >= http.StatusInternalServerError || response.StatusCode == http.StatusTooManyRequests

		return nil, retriable, fmt.Errorf("get %s: unexpected status %s", url, response.Status)
	}

	return body, false, nil
}
