// Command denolock pins the Deno release nelm runs TypeScript charts with: it downloads every
// platform's release archive, records its sha256 and the sha256 of the binary inside it, and writes
// pkg/ts/denolock/data/lock.json, which is committed and embedded into the binary. The generated
// lock is the only place the Deno version is configured. See pkg/ts/denolock/data/README.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-resty/resty/v2"

	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts/denolock"
	"github.com/werf/nelm/pkg/util"
)

const (
	defaultOutputPath  = "pkg/ts/denolock/data/lock.json"
	defaultParallelism = 5
	latestReleaseURL   = "https://api.github.com/repos/denoland/deno/releases/latest"
	requestTimeout     = 15 * time.Minute
)

// targets is the canonical platform list: the lock is generated from it and -verify checks the
// committed lock against it. Adding a platform also needs a pkg/ts/embed_<os>_<arch>.go.
var targets = map[string]string{
	"darwin/amd64":  "x86_64-apple-darwin",
	"darwin/arm64":  "aarch64-apple-darwin",
	"linux/amd64":   "x86_64-unknown-linux-gnu",
	"linux/arm64":   "aarch64-unknown-linux-gnu",
	"windows/amd64": "x86_64-pc-windows-msvc",
}

type options struct {
	checkUpstream bool
	outputPath    string
	parallelism   int
	verify        bool
	version       string
}

type platformResult struct {
	archiveSize int64
	entry       *denolock.Platform
	platform    string
}

func run(ctx context.Context) error {
	opts := parseFlags()

	if opts.verify {
		return verifyLock(ctx, opts)
	}

	client := util.NewRestyClient(ctx)
	client.SetTimeout(requestTimeout)

	if opts.checkUpstream {
		return checkUpstream(ctx, client, opts)
	}

	// Zero would deadlock the download semaphore and a negative value would panic building it.
	if opts.parallelism < 1 {
		return fmt.Errorf("parallelism must be positive, got %d", opts.parallelism)
	}

	return generate(ctx, client, opts)
}

func generate(ctx context.Context, client *resty.Client, opts options) error {
	version, err := resolveVersion(opts)
	if err != nil {
		return err
	}

	log.Default.Info(ctx, "Pinning Deno %s for %d platforms", version, len(targets))

	platforms := slices.Sorted(maps.Keys(targets))
	results := make([]*platformResult, len(platforms))
	errs := make([]error, len(platforms))

	semaphore := make(chan struct{}, opts.parallelism)

	var wg sync.WaitGroup

	for i, platform := range platforms {
		wg.Add(1)

		go func() {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := pinPlatform(ctx, client, version, platform)
			if err != nil {
				errs[i] = fmt.Errorf("pin %s: %w", platform, err)

				return
			}

			results[i] = result
		}()
	}

	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return err
	}

	lock := &denolock.Lock{
		Platforms: make(map[string]*denolock.Platform, len(results)),
		Version:   version,
	}

	for _, result := range results {
		lock.Platforms[result.platform] = result.entry

		log.Default.Info(ctx, "Pinned %s: archive %s (%s), binary %s",
			result.platform, result.entry.ArchiveSHA256[:12], humanize.IBytes(uint64(result.archiveSize)),
			result.entry.BinarySHA256[:12])
	}

	if err := lock.Validate(); err != nil {
		return fmt.Errorf("generated lock is invalid: %w", err)
	}

	if err := writeLock(opts.outputPath, lock); err != nil {
		return err
	}

	log.Default.Info(ctx, "Wrote %s: review the digests in the diff, that is what the pinning is for", opts.outputPath)

	return nil
}

func pinPlatform(ctx context.Context, client *resty.Client, version, platform string) (*platformResult, error) {
	goos, _, _ := strings.Cut(platform, "/")
	target := targets[platform]
	archiveURL := denolock.ArchiveURLFor(version, target)

	log.Default.Info(ctx, "Downloading %s", archiveURL)

	archive, err := get(ctx, client, archiveURL)
	if err != nil {
		return nil, err
	}

	archiveSHA256 := sha256Hex(archive)

	// Useless as a trust anchor, but it is the only thing that can tell an intact download from a
	// broken one at the moment the digest is taken.
	upstreamSHA256, err := fetchUpstreamChecksum(ctx, client, archiveURL)
	if err != nil {
		return nil, err
	}

	if upstreamSHA256 != archiveSHA256 {
		return nil, fmt.Errorf("downloaded archive hashes to %s, but upstream publishes %s: refusing to pin a download upstream does not vouch for",
			archiveSHA256, upstreamSHA256)
	}

	binary, err := extractDenoBinary(archive, goos)
	if err != nil {
		return nil, err
	}

	return &platformResult{
		archiveSize: int64(len(archive)),
		entry: &denolock.Platform{
			ArchiveSHA256: archiveSHA256,
			BinarySHA256:  sha256Hex(binary),
			Target:        target,
		},
		platform: platform,
	}, nil
}

// checkUpstream reports whether the release assets the lock pins still hash to what it records:
// GitHub lets an asset be replaced without moving its tag. It compares the published checksums
// rather than downloading a few hundred megabytes, which is enough to notice a swap but is not
// verification — that is the lock's job.
func checkUpstream(ctx context.Context, client *resty.Client, opts options) error {
	lock, err := readLock(opts.outputPath)
	if err != nil {
		return err
	}

	var drifted []string

	for _, platform := range slices.Sorted(maps.Keys(lock.Platforms)) {
		entry := lock.Platforms[platform]
		archiveURL := denolock.ArchiveURLFor(lock.Version, entry.Target)

		upstreamSHA256, err := fetchUpstreamChecksum(ctx, client, archiveURL)
		if err != nil {
			return err
		}

		if upstreamSHA256 != entry.ArchiveSHA256 {
			drifted = append(drifted, fmt.Sprintf("%s now publishes %s, the lock pins %s", platform, upstreamSHA256, entry.ArchiveSHA256))
		}
	}

	if len(drifted) > 0 {
		return fmt.Errorf("the Deno %s release assets changed after the lock was generated: %s. Nothing legitimate replaces a published release asset: investigate before running \"task deno:lock\"",
			lock.Version, strings.Join(drifted, "; "))
	}

	// Not drift, just a bump available, so it is reported rather than failed on.
	if latest, err := latestVersion(ctx, client); err != nil {
		log.Default.Warn(ctx, "Could not check for a newer Deno release: %s", err)
	} else if latest != lock.Version {
		log.Default.Info(ctx, "Deno %s is available upstream, pinned is %s", latest, lock.Version)
	}

	log.Default.Info(ctx, "The Deno %s release assets still match the lock, for all %d platforms", lock.Version, len(lock.Platforms))

	return nil
}

// verifyLock checks the committed lock offline. Whether the digests describe the real Deno release
// needs the network, which is what -check-upstream is for.
func verifyLock(ctx context.Context, opts options) error {
	lock, err := readLock(opts.outputPath)
	if err != nil {
		return err
	}

	want := slices.Sorted(maps.Keys(targets))
	got := slices.Sorted(maps.Keys(lock.Platforms))

	if !slices.Equal(want, got) {
		return fmt.Errorf("%s pins %s, but nelm builds for %s, run: task deno:lock",
			opts.outputPath, strings.Join(got, ", "), strings.Join(want, ", "))
	}

	for _, platform := range got {
		if recorded, want := lock.Platforms[platform].Target, targets[platform]; recorded != want {
			return fmt.Errorf("%s records %s as target %q, but it is %q, run: task deno:lock",
				opts.outputPath, platform, recorded, want)
		}
	}

	log.Default.Info(ctx, "The Deno lock is consistent: %s, for %s", lock.Version, strings.Join(got, ", "))

	return nil
}

func resolveVersion(opts options) (string, error) {
	if opts.version != "" {
		return strings.TrimPrefix(opts.version, "v"), nil
	}

	lock, err := readLock(opts.outputPath)
	if err != nil {
		return "", fmt.Errorf("%w, and no -version given to pin instead", err)
	}

	return lock.Version, nil
}

// readLock reads the file in the working tree, not the copy embedded into this binary.
func readLock(path string) (*denolock.Lock, error) {
	lockBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var lock denolock.Lock

	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	if err := lock.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}

	return &lock, nil
}

func writeLock(path string, lock *denolock.Lock) error {
	lockBytes, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lock: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, append(lockBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func parseFlags() options {
	var opts options

	flag.BoolVar(&opts.checkUpstream, "check-upstream", false,
		"Report whether the release assets the lock pins still hash to what it records, and exit")
	flag.StringVar(&opts.outputPath, "output", defaultOutputPath, "Lock file to write, verify or check")
	flag.IntVar(&opts.parallelism, "parallelism", defaultParallelism, "How many platforms to download concurrently")
	flag.BoolVar(&opts.verify, "verify", false,
		"Check the committed lock against the platforms nelm builds for and exit, without downloading anything")
	flag.StringVar(&opts.version, "version", "",
		"Deno version to pin, with or without the leading \"v\". Defaults to the version the lock already records")
	flag.Parse()

	return opts
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		log.Default.Error(ctx, "Error: %s", err)
		os.Exit(1)
	}
}
