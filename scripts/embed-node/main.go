// Command embed-node downloads the pinned Node.js release for one platform and packs its binary into
// pkg/ts/embed/<os>/<arch>/node.gz and its npm package into pkg/ts/embed/<os>/<arch>/npm.tar.gz,
// which the embednode build tag compiles into the release binary. The blobs are not committed, so
// what makes them the Node.js this repository pinned is pkg/ts/nodelock, which both the download here
// and the extraction at run time verify against.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/ts"
	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

func run() error {
	embedRoot := os.Getenv("NODE_EMBED_ROOT")
	if embedRoot == "" {
		embedRoot = filepath.Join("pkg", "ts", "embed")
	}

	platform := os.Getenv("NODE_EMBED_PLATFORM")
	if platform == "" {
		return fmt.Errorf("NODE_EMBED_PLATFORM is required: want <os>/<arch>")
	}

	goos, goarch, found := strings.Cut(platform, "/")
	if !found {
		return fmt.Errorf("parse NODE_EMBED_PLATFORM %q: want <os>/<arch>", platform)
	}

	if _, err := nodelock.Get(goos, goarch); err != nil {
		return fmt.Errorf("unsupported NODE_EMBED_PLATFORM %q: %w", platform, err)
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

	downloadDir, err := os.MkdirTemp("", "embed-node-*")
	if err != nil {
		return fmt.Errorf("create temp download directory: %w", err)
	}

	defer os.RemoveAll(downloadDir)

	nodePath, npmDir, err := ts.DownloadNodeForPlatform(ctx, goos, goarch, downloadDir)
	if err != nil {
		return fmt.Errorf("download Node.js: %w", err)
	}

	if err := writeAtomically(filepath.Join(platformDir, "node.gz"), func(writer io.Writer) error {
		return gzipFile(nodePath, writer)
	}); err != nil {
		return fmt.Errorf("pack Node.js binary: %w", err)
	}

	if err := writeAtomically(filepath.Join(platformDir, "npm.tar.gz"), func(writer io.Writer) error {
		return tarGzDir(npmDir, writer)
	}); err != nil {
		return fmt.Errorf("pack npm: %w", err)
	}

	version, err := nodelock.Version()
	if err != nil {
		return fmt.Errorf("get the pinned Node.js version: %w", err)
	}

	log.Default.Info(ctx, "Embedded Node.js %s for %s/%s", version, goos, goarch)

	return nil
}

// tarGzDir packs the regular files of dir with paths relative to it, in a stable order and without
// timestamps or owners, so that the same npm package always gives the same blob.
func tarGzDir(dir string, writer io.Writer) error {
	var paths []string

	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected non-regular file %s", path)
		}

		paths = append(paths, path)

		return nil
	}); err != nil {
		return fmt.Errorf("walk %s: %w", dir, err)
	}

	slices.SortFunc(paths, func(a, b string) int {
		return strings.Compare(filepath.ToSlash(a), filepath.ToSlash(b))
	})

	gzWriter := gzip.NewWriter(writer)
	tarWriter := tar.NewWriter(gzWriter)

	for _, path := range paths {
		if err := addTarFile(tarWriter, dir, path); err != nil {
			return err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}

	return nil
}

func addTarFile(tarWriter *tar.Writer, dir, path string) error {
	relPath, err := filepath.Rel(dir, path)
	if err != nil {
		return fmt.Errorf("get path of %s relative to %s: %w", path, dir, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	mode := int64(0o644)
	if info.Mode()&0o111 != 0 {
		mode = 0o755
	}

	if err := tarWriter.WriteHeader(&tar.Header{
		Mode:     mode,
		Name:     filepath.ToSlash(relPath),
		Size:     info.Size(),
		Typeflag: tar.TypeReg,
	}); err != nil {
		return fmt.Errorf("write tar header for %s: %w", relPath, err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	defer file.Close()

	if _, err := io.Copy(tarWriter, file); err != nil {
		return fmt.Errorf("pack %s: %w", relPath, err)
	}

	return nil
}

func gzipFile(path string, writer io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	defer file.Close()

	gzWriter := gzip.NewWriter(writer)

	if _, err := io.Copy(gzWriter, file); err != nil {
		return fmt.Errorf("compress %s: %w", path, err)
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}

	return nil
}

func writeAtomically(path string, write func(writer io.Writer) error) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}

	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := write(tmpFile); err != nil {
		tmpFile.Close()

		return err
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Default.Error(context.Background(), "embed-node: %v", err)
		os.Exit(1)
	}
}
