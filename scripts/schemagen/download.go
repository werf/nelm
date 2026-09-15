package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/werf/nelm/pkg/log"
)

const downloadAttempts = 3

// downloadMergedKubernetesSchemas collects the passed versions into a single flat set, newest first,
// each contributing only the schemas none of the newer ones has. Only those are downloaded, which is
// what makes reaching further back cheap: consecutive minor versions differ by a handful of files.
func downloadMergedKubernetesSchemas(ctx context.Context, client *http.Client, opts options, kubeVersions []string, dirSHAs map[string]string, commit string) ([]schemaFile, error) {
	var files []schemaFile

	seen := make(map[string]struct{})

	for _, kubeVersion := range kubeVersions {
		dirName := kubernetesVersionDirName(kubeVersion)

		dirSHA, ok := dirSHAs[dirName]
		if !ok {
			return nil, fmt.Errorf("%s has no %s directory at %s", opts.kubernetesRepo, dirName, commit)
		}

		names, err := listKubernetesSchemas(ctx, client, opts, dirName, dirSHA)
		if err != nil {
			return nil, fmt.Errorf("list schemas of Kubernetes %s: %w", kubeVersion, err)
		}

		missing := make([]string, 0, len(names))

		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}

			seen[name] = struct{}{}

			missing = append(missing, name)
		}

		if len(missing) == 0 {
			log.Default.Info(ctx, "Kubernetes %s has no schemas that a newer version does not have already", kubeVersion)

			continue
		}

		log.Default.Info(ctx, "Downloading %d of the %d schemas of Kubernetes %s from %s@%s",
			len(missing), len(names), kubeVersion, opts.kubernetesRepo, commit[:7])

		downloaded, err := downloadKubernetesSchemas(ctx, client, opts, kubeVersion, commit, missing)
		if err != nil {
			return nil, fmt.Errorf("download schemas of Kubernetes %s: %w", kubeVersion, err)
		}

		files = append(files, downloaded...)
	}

	return files, nil
}

func downloadKubernetesSchemas(ctx context.Context, client *http.Client, opts options, kubeVersion, commit string, names []string) ([]schemaFile, error) {
	// Cancelling on the first failure keeps a dead or rate-limiting server from being hammered with
	// every remaining name before the error surfaces.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	files := make([]schemaFile, len(names))
	errs := make([]error, len(names))
	semaphore := make(chan struct{}, opts.parallelism)

	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)

		go func() {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			file, err := downloadKubernetesSchema(ctx, client, opts, kubeVersion, commit, name)
			if err != nil {
				// Requests we cancelled ourselves would only bury the error that caused it.
				if ctx.Err() == nil {
					errs[i] = err
				}

				cancel()

				return
			}

			files[i] = file
		}()
	}

	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return files, nil
}

// kubernetesVersionDirSHAs lists the directories of the repository at treeish, which may be a commit
// or a ref, mapped to the tree sha their contents are listed by. One request serves every version.
func kubernetesVersionDirSHAs(ctx context.Context, client *http.Client, repo, treeish string) (map[string]string, error) {
	var rootTree struct {
		Tree []struct {
			Path string `json:"path"`
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"tree"`
	}

	rootURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s", repo, treeish)
	if err := getJSON(ctx, client, rootURL, &rootTree); err != nil {
		return nil, err
	}

	dirSHAs := make(map[string]string, len(rootTree.Tree))

	for _, entry := range rootTree.Tree {
		if entry.Type == "tree" {
			dirSHAs[entry.Path] = entry.SHA
		}
	}

	if len(dirSHAs) == 0 {
		return nil, fmt.Errorf("%s has no directories at %s", repo, treeish)
	}

	return dirSHAs, nil
}

// listKubernetesSchemas returns the names kubeconform can actually request. Upstream ships every
// schema twice, bare and suffixed with group and version; kubeconform only ever builds the suffixed
// name, so the bare half of the tree and the shared "_definitions.json" are dead weight.
func listKubernetesSchemas(ctx context.Context, client *http.Client, opts options, dirName, dirSHA string) ([]string, error) {
	var dirTree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}

	dirURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s", opts.kubernetesRepo, dirSHA)
	if err := getJSON(ctx, client, dirURL, &dirTree); err != nil {
		return nil, err
	}

	if dirTree.Truncated {
		return nil, fmt.Errorf("listing of %s was truncated by the GitHub API", dirName)
	}

	var names []string

	for _, entry := range dirTree.Tree {
		if entry.Type != "blob" || !strings.HasSuffix(entry.Path, ".json") {
			continue
		}

		// Every name kubeconform builds carries at least a "-<version>" suffix.
		if !strings.Contains(entry.Path, "-") {
			continue
		}

		names = append(names, entry.Path)
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("no schemas found in %s", dirName)
	}

	slices.Sort(names)

	return names, nil
}

func downloadKubernetesSchema(ctx context.Context, client *http.Client, opts options, kubeVersion, commit, name string) (schemaFile, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/v%s-standalone/%s",
		opts.kubernetesRepo, commit, kubeVersion, name)

	content, err := get(ctx, client, url, nil)
	if err != nil {
		return schemaFile{}, fmt.Errorf("download %s: %w", name, err)
	}

	stripped, err := stripSchemaBytes(content)
	if err != nil {
		return schemaFile{}, fmt.Errorf("strip %s: %w", name, err)
	}

	return schemaFile{content: stripped, name: name}, nil
}

func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	body, err := get(ctx, client, url, nil)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response of %s: %w", url, err)
	}

	return nil
}

func resolveCommit(ctx context.Context, client *http.Client, repo, ref string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, ref)

	// This Accept header makes the API respond with the bare commit sha instead of the full commit.
	body, err := get(ctx, client, url, map[string]string{"Accept": "application/vnd.github.sha"})
	if err != nil {
		return "", err
	}

	commit := strings.TrimSpace(string(body))
	if len(commit) < 7 {
		return "", fmt.Errorf("got unexpected commit sha %q", commit)
	}

	return commit, nil
}

// downloadCRDsCatalog takes the catalog as one repository tarball and keeps what kubeconform can
// address: the schemas laid out as "<group>/<file>.json". Anything deeper, such as the
// "openshift/v4.11-strict/" trees, is unreachable through the path template and would only bloat.
func downloadCRDsCatalog(ctx context.Context, client *http.Client, repo, commit string) ([]schemaFile, error) {
	url := fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", repo, commit)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", url, err)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: unexpected status %s", url, response.Status)
	}

	gzipReader, err := gzip.NewReader(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s as gzip: %w", url, err)
	}

	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	var files []schemaFile

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read %s as tar: %w", url, err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		name, ok := crdSchemaName(header.Name)
		if !ok {
			continue
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", header.Name, err)
		}

		stripped, err := stripSchemaBytes(content)
		if err != nil {
			return nil, fmt.Errorf("strip %s: %w", header.Name, err)
		}

		files = append(files, schemaFile{content: stripped, name: name})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no CRD schemas found in %s", url)
	}

	return files, nil
}

func get(ctx context.Context, client *http.Client, url string, headers map[string]string) ([]byte, error) {
	var lastErr error

	for attempt := range downloadAttempts {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		body, retriable, err := getOnce(ctx, client, url, headers)
		if err == nil {
			return body, nil
		}

		lastErr = err

		if !retriable {
			break
		}
	}

	return nil, lastErr
}

// buildArchive packs the schemas into a reproducible tar.gz: entries sorted by name and every piece of
// metadata that would vary between machines zeroed out, so the same inputs give identical bytes.
func buildArchive(files []schemaFile) ([]byte, int64, error) {
	slices.SortFunc(files, func(a, b schemaFile) int {
		return strings.Compare(a.name, b.name)
	})

	var buf bytes.Buffer

	gzipWriter, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, 0, fmt.Errorf("create gzip writer: %w", err)
	}

	tarWriter := tar.NewWriter(gzipWriter)

	var uncompressedSize int64

	for _, file := range files {
		header := &tar.Header{
			Format:   tar.FormatUSTAR,
			ModTime:  time.Unix(0, 0).UTC(),
			Mode:     0o644,
			Name:     file.name,
			Size:     int64(len(file.content)),
			Typeflag: tar.TypeReg,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, 0, fmt.Errorf("write header for %s: %w", file.name, err)
		}

		if _, err := tarWriter.Write(file.content); err != nil {
			return nil, 0, fmt.Errorf("write %s: %w", file.name, err)
		}

		uncompressedSize += int64(len(file.content))
	}

	if err := tarWriter.Close(); err != nil {
		return nil, 0, fmt.Errorf("close tar writer: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, 0, fmt.Errorf("close gzip writer: %w", err)
	}

	return buf.Bytes(), uncompressedSize, nil
}

// crdSchemaName turns a path inside the catalog tarball into the bundle entry name, reporting
// whether the file is a schema kubeconform can address at all.
func crdSchemaName(tarPath string) (string, bool) {
	// Repository tarballs nest everything under a single "<repo>-<commit>/" directory.
	_, path, found := strings.Cut(tarPath, "/")
	if !found {
		return "", false
	}

	if !strings.HasSuffix(path, ".json") {
		return "", false
	}

	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", false
	}

	// The schema path template always renders "<kind>_<version>.json".
	if !strings.Contains(parts[1], "_") {
		return "", false
	}

	return path, true
}

func getOnce(ctx context.Context, client *http.Client, url string, headers map[string]string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create request for %s: %w", url, err)
	}

	for name, value := range headers {
		request.Header.Set(name, value)
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

func kubernetesVersionDirName(kubeVersion string) string {
	return "v" + kubeVersion + "-standalone"
}
