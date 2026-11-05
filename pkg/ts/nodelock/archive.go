package nodelock

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

type ReadArchiveOptions struct {
	// OnBinary is called with the content of the Node.js binary.
	OnBinary func(content io.Reader) error
	// OnNPMFile is called for every regular file of the npm package, with its slash-separated path
	// relative to the package root.
	OnNPMFile func(path string, mode fs.FileMode, content io.Reader) error
}

type archiveReader struct {
	binaryFound bool
	binaryPath  string
	npmFound    bool
	npmPrefix   string
	opts        ReadArchiveOptions
}

func (r *archiveReader) npmPath(name string) string {
	npmPath, found := strings.CutPrefix(stripTopDir(name), r.npmPrefix)
	if !found {
		return ""
	}

	return npmPath
}

func (r *archiveReader) readFile(name string, mode fs.FileMode, content io.Reader) error {
	if stripTopDir(name) == r.binaryPath {
		r.binaryFound = true

		if err := r.opts.OnBinary(content); err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		return nil
	}

	npmPath := r.npmPath(name)
	if npmPath == "" {
		return nil
	}

	if !fs.ValidPath(npmPath) || strings.Contains(npmPath, "\n") {
		return fmt.Errorf("unsafe path %q in the npm package", name)
	}

	r.npmFound = true

	if err := r.opts.OnNPMFile(npmPath, mode, content); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	return nil
}

func (r *archiveReader) readTarGz(archive io.Reader) error {
	gzReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}

	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			if err := r.readFile(header.Name, header.FileInfo().Mode(), tarReader); err != nil {
				return err
			}
		case tar.TypeDir:
		default:
			if r.npmPath(header.Name) != "" {
				return fmt.Errorf("unexpected entry %q of type %q in the npm package", header.Name, header.Typeflag)
			}
		}
	}
}

func (r *archiveReader) readZip(archive io.ReaderAt, size int64) error {
	zipReader, err := zip.NewReader(archive, size)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}

	for _, file := range zipReader.File {
		mode := file.Mode()
		if mode.IsDir() {
			continue
		}

		if !mode.IsRegular() {
			if r.npmPath(file.Name) != "" {
				return fmt.Errorf("unexpected entry %q of mode %s in the npm package", file.Name, mode)
			}

			continue
		}

		if err := r.readZipFile(file); err != nil {
			return err
		}
	}

	return nil
}

func (r *archiveReader) readZipFile(file *zip.File) error {
	content, err := file.Open()
	if err != nil {
		return fmt.Errorf("open %s in archive: %w", file.Name, err)
	}

	defer content.Close()

	return r.readFile(file.Name, file.Mode(), content)
}

// ReadArchive reads the Node.js binary and the npm package from an official Node.js release archive:
// a .zip for windows, a .tar.gz for everything else.
func ReadArchive(archive io.ReaderAt, size int64, goos string, opts ReadArchiveOptions) error {
	reader := &archiveReader{
		binaryPath: "bin/node",
		npmPrefix:  "lib/node_modules/npm/",
		opts:       opts,
	}

	if goos == "windows" {
		reader.binaryPath = "node.exe"
		reader.npmPrefix = "node_modules/npm/"

		if err := reader.readZip(archive, size); err != nil {
			return err
		}
	} else if err := reader.readTarGz(io.NewSectionReader(archive, 0, size)); err != nil {
		return err
	}

	if !reader.binaryFound {
		return fmt.Errorf("no %s in archive", reader.binaryPath)
	}

	if !reader.npmFound {
		return fmt.Errorf("no %s in archive", reader.npmPrefix)
	}

	return nil
}

// stripTopDir drops the "node-v<version>-<target>/" directory every entry of a release archive is in.
func stripTopDir(name string) string {
	_, rest, found := strings.Cut(name, "/")
	if !found {
		return ""
	}

	return rest
}
