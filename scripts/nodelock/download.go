package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/go-resty/resty/v2"

	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

const (
	releaseIndexURL = "https://nodejs.org/dist/index.json"
	sha256HexLen    = 64
)

type archiveContents struct {
	binarySHA256   string
	npmFileSHA256s map[string]string
	npmVersion     string
}

func fetchUpstreamChecksums(ctx context.Context, client *resty.Client, version string) (map[string]string, error) {
	checksumsURL := nodelock.DistURLFor(version) + nodelock.ChecksumsFileName

	body, err := get(ctx, client, checksumsURL)
	if err != nil {
		return nil, err
	}

	checksums, err := parseChecksums(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", checksumsURL, err)
	}

	return checksums, nil
}

// latestVersionInLine returns the newest release of the major line the given version belongs to:
// moving to the next major line is a decision, not a routine bump.
func latestVersionInLine(ctx context.Context, client *resty.Client, version string) (string, error) {
	body, err := get(ctx, client, releaseIndexURL)
	if err != nil {
		return "", err
	}

	var releases []struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("decode response of %s: %w", releaseIndexURL, err)
	}

	major, _, _ := strings.Cut(version, ".")

	// The index lists the newest release first.
	for _, release := range releases {
		releaseVersion := strings.TrimPrefix(release.Version, "v")
		if releaseMajor, _, _ := strings.Cut(releaseVersion, "."); releaseMajor == major {
			return releaseVersion, nil
		}
	}

	return "", fmt.Errorf("%s lists no release of Node.js %s", releaseIndexURL, major)
}

func readArchive(archive []byte, goos string) (*archiveContents, error) {
	contents := &archiveContents{npmFileSHA256s: map[string]string{}}

	err := nodelock.ReadArchive(bytes.NewReader(archive), int64(len(archive)), goos, nodelock.ReadArchiveOptions{
		OnBinary: func(content io.Reader) error {
			digest, err := sha256HexReader(content)
			contents.binarySHA256 = digest

			return err
		},
		OnNPMFile: func(path string, _ fs.FileMode, content io.Reader) error {
			data, err := io.ReadAll(content)
			if err != nil {
				return err
			}

			contents.npmFileSHA256s[path] = sha256Hex(data)

			if path == "package.json" {
				var packageJSON struct {
					Version string `json:"version"`
				}

				if err := json.Unmarshal(data, &packageJSON); err != nil {
					return fmt.Errorf("decode: %w", err)
				}

				contents.npmVersion = packageJSON.Version
			}

			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	if contents.npmVersion == "" {
		return nil, fmt.Errorf("no version in the package.json of npm in archive")
	}

	return contents, nil
}

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

// parseChecksums parses sha256sum output, "<hex>  <file>" per line, into digests keyed by file name.
func parseChecksums(body string) (map[string]string, error) {
	checksums := map[string]string{}

	for line := range strings.Lines(body) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed line %q", strings.TrimSpace(line))
		}

		digest := strings.ToLower(fields[0])
		if _, err := hex.DecodeString(digest); err != nil || len(digest) != sha256HexLen {
			return nil, fmt.Errorf("malformed digest in line %q", strings.TrimSpace(line))
		}

		checksums[fields[1]] = digest
	}

	if len(checksums) == 0 {
		return nil, fmt.Errorf("no checksums")
	}

	return checksums, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

func sha256HexReader(reader io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
