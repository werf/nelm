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
	"strings"

	"github.com/go-resty/resty/v2"

	"github.com/werf/nelm/pkg/ts/denolock"
)

const sha256HexLen = 64

func get(ctx context.Context, client *resty.Client, url string) ([]byte, error) {
	response, err := client.R().SetContext(ctx).Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}

	if response.IsError() {
		return nil, fmt.Errorf("get %s: %s", url, response.Status())
	}

	return response.Body(), nil
}

func fetchUpstreamChecksum(ctx context.Context, client *resty.Client, archiveURL string) (string, error) {
	checksumURL := archiveURL + denolock.ChecksumURLSuffix

	body, err := get(ctx, client, checksumURL)
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

func latestVersion(ctx context.Context, client *resty.Client) (string, error) {
	body, err := get(ctx, client, latestReleaseURL)
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
