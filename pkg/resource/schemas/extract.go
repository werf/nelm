package schemas

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/log"
)

const (
	// The depths allow the CRDs catalog its one "<group>/" level and keep the Kubernetes bundle flat.
	crdsMaxPathDepth       = 2
	extractionLockFileName = ".lock"
	extractionTempPattern  = "extract-*"
	kubernetesMaxPathDepth = 1
	// Generous on purpose: another nelm process may still be validating against the old extraction.
	staleExtractionLifetime = 30 * 24 * time.Hour
)

var (
	extractionMu sync.Mutex

	extractedDirs = make(map[string]struct{})
)

// embeddedBundle is one archive embedded into the binary, with how it is laid out once unpacked.
type embeddedBundle struct {
	archiveFileName    string
	index              *Bundle
	maxPathDepth       int
	name               string
	schemaPathTemplate string
}

// ensureExtracted unpacks the bundle to the cache directory and returns that directory. Bundles go to
// disk rather than memory, at most once per process.
func (b *embeddedBundle) ensureExtracted(ctx context.Context) (string, error) {
	dir := b.extractionDir()

	extractionMu.Lock()
	defer extractionMu.Unlock()

	if _, ok := extractedDirs[dir]; ok {
		return dir, nil
	}

	if err := b.extract(ctx, dir); err != nil {
		return "", err
	}

	extractedDirs[dir] = struct{}{}

	return dir, nil
}

func (b *embeddedBundle) extract(ctx context.Context, dir string) error {
	if extracted, err := b.isExtracted(dir); err != nil {
		return err
	} else if extracted {
		return nil
	}

	root := filepath.Dir(dir)

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create cache dir %q: %w", root, err)
	}

	fileLock := flock.New(filepath.Join(root, extractionLockFileName))

	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on %s: %w", fileLock.Path(), err)
	}

	defer func() {
		if err := fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on %s: %s", fileLock.Path(), err)
		}
	}()

	// Another process might have unpacked the bundle while we were waiting for the lock.
	if extracted, err := b.isExtracted(dir); err != nil {
		return err
	} else if extracted {
		return nil
	}

	log.Default.Debug(ctx, "Unpacking %d embedded %s schemas, %.1f MiB, to %s",
		b.index.FilesCount, b.name, float64(b.index.UncompressedSize)/1024/1024, dir)

	tmpDir, err := os.MkdirTemp(root, extractionTempPattern)
	if err != nil {
		return fmt.Errorf("create temp dir in %q: %w", root, err)
	}

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	if err := b.unpack(tmpDir); err != nil {
		return fmt.Errorf("unpack embedded %s schemas: %w", b.name, err)
	}

	// An incomplete directory has to go first: renaming onto a non-empty one fails.
	if present, err := isDir(dir); err != nil {
		return err
	} else if present {
		log.Default.Debug(ctx, "Replacing incomplete embedded %s schemas at %s", b.name, dir)

		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove incomplete %q: %w", dir, err)
		}
	}

	// Renaming keeps the bundle either wholly absent or wholly usable, even if we are killed midway.
	if err := os.Rename(tmpDir, dir); err != nil {
		if extracted, checkErr := b.isExtracted(dir); checkErr != nil || !extracted {
			return fmt.Errorf("move %q to %q: %w", tmpDir, dir, err)
		}
	}

	removeStaleExtractions(ctx, root)

	return nil
}

// extractionDir carries the archive digest in its name, so an upgrade shipping a regenerated bundle
// unpacks next to the old one instead of mutating what other nelm processes may still be reading.
func (b *embeddedBundle) extractionDir() string {
	return filepath.Join(helmpath.CachePath(common.CacheDirEmbeddedAPIResourceJSONSchemas), extractionDirName(b.name, b.index))
}

// isExtracted counts the files against the index rather than just stat'ing the directory: one that is
// there but has lost files makes kubeconform find no schema, and validation then passes silently.
// Cache cleaners deleting by age, as systemd-tmpfiles does to ~/.cache, produce exactly that.
func (b *embeddedBundle) isExtracted(dir string) (bool, error) {
	if present, err := isDir(dir); err != nil || !present {
		return false, err
	}

	filesCount, err := countFiles(dir)
	if err != nil {
		return false, err
	}

	return filesCount == b.index.FilesCount, nil
}

func (b *embeddedBundle) unpack(destDir string) error {
	archiveFile, err := data.Open(b.archiveFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", b.archiveFileName, err)
	}

	defer archiveFile.Close()

	return unpackArchive(destDir, archiveFile, b.maxPathDepth)
}

func unpackArchive(destDir string, reader io.Reader, maxPathDepth int) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("read bundle as gzip: %w", err)
	}

	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("read bundle as tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unexpected non-regular entry %q in bundle", header.Name)
		}

		schemaPath, err := schemaEntryPath(destDir, header.Name, maxPathDepth)
		if err != nil {
			return err
		}

		if err := writeSchemaFile(schemaPath, tarReader); err != nil {
			return err
		}
	}

	return nil
}

func countFiles(dir string) (int, error) {
	var count int

	if err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.Type().IsRegular() {
			count++
		}

		return nil
	}); err != nil {
		return 0, fmt.Errorf("count files in %q: %w", dir, err)
	}

	return count, nil
}

func isDir(dir string) (bool, error) {
	stat, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat %q: %w", dir, err)
	}

	if !stat.IsDir() {
		return false, fmt.Errorf("%s is not a directory", dir)
	}

	return true, nil
}

func newBundle(name, archiveFileName, schemaPathTemplate string, maxPathDepth int, index *Bundle) *embeddedBundle {
	return &embeddedBundle{
		archiveFileName:    archiveFileName,
		index:              index,
		maxPathDepth:       maxPathDepth,
		name:               name,
		schemaPathTemplate: schemaPathTemplate,
	}
}

// removeStaleExtractions cleans up bundles of other nelm versions and temp directories of interrupted
// extractions. Best effort: failing to clean up must never fail validation.
//
// Every bundle this binary carries is kept, not just the one extracted last: a release that
// regenerates one bundle and leaves the other alone would otherwise delete the other's live
// extraction, once it has aged past staleExtractionLifetime.
func removeStaleExtractions(ctx context.Context, root string) {
	keep, err := liveExtractionDirNames()
	if err != nil {
		log.Default.Debug(ctx, "Cannot tell live embedded schemas apart from stale ones, skipping cleanup: %s", err)

		return
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		log.Default.Debug(ctx, "Cannot list %s to clean up stale embedded schemas: %s", root, err)

		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || slices.Contains(keep, entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleExtractionLifetime {
			continue
		}

		path := filepath.Join(root, entry.Name())

		log.Default.Debug(ctx, "Removing stale embedded schemas %s", path)

		if err := os.RemoveAll(path); err != nil {
			log.Default.Debug(ctx, "Cannot remove stale embedded schemas %s: %s", path, err)
		}
	}
}

// schemaEntryPath turns a bundle entry name into a path inside destDir. Our bundles hold nothing but
// schema files at a known depth, so an entry resolving anywhere else means this is not one of ours.
func schemaEntryPath(destDir, entryName string, maxPathDepth int) (string, error) {
	if strings.ContainsRune(entryName, '\\') {
		return "", fmt.Errorf("unexpected entry name %q in bundle", entryName)
	}

	parts := strings.Split(entryName, "/")
	if len(parts) > maxPathDepth {
		return "", fmt.Errorf("unexpected entry name %q in bundle: at most %d path components allowed", entryName, maxPathDepth)
	}

	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unexpected entry name %q in bundle", entryName)
		}
	}

	path := filepath.Join(append([]string{destDir}, parts...)...)
	if !strings.HasPrefix(path, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("unexpected entry name %q in bundle", entryName)
	}

	return path, nil
}

func writeSchemaFile(path string, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %q: %w", filepath.Dir(path), err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}

	if _, err := io.Copy(file, reader); err != nil {
		file.Close()

		return fmt.Errorf("write %q: %w", path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}

	return nil
}
