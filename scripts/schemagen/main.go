// Command schemagen downloads the JSON schemas embedded into the nelm binary, strips them down and
// packs them into the archives read by pkg/resource/schemas: the Kubernetes API schemas of
// -kube-version merged with those of the minor versions before it at their latest patch, and the CRDs
// catalog.
//
// The version is pinned by "kubeconformValidationSchemasUpstreamKubeVersion" in the Taskfile, the one
// place it is configured; the generated index is what tells nelm which version it validates against.
// Runs as a build dependency and is idempotent: an archive matching what the index records is left
// alone.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resource/schemas"
)

const (
	crdsArchiveFileName = "crds.tar.gz"
	defaultCRDsRef      = "main"
	defaultCRDsRepo     = "datreeio/CRDs-catalog"
	// Older versions only contribute the schemas newer ones no longer have, so each one past the first
	// costs next to nothing.
	defaultKubeVersionCount = 5
	defaultKubernetesRef    = "master"
	defaultKubernetesRepo   = "yannh/kubernetes-json-schema"
	// defaultMaxBundlesSize caps what the archives may grow to. Every byte of them is carried by every
	// nelm binary and by every module that imports nelm, so the size is a deliberate budget rather than
	// whatever upstream happens to produce.
	defaultMaxBundlesSize     = "5MiB"
	defaultOutputDir          = "pkg/resource/schemas/data"
	defaultParallelism        = 16
	indexFileName             = "index.json"
	kubernetesArchiveFileName = "kubernetes.tar.gz"
	// The per version archives the merged bundle replaced, which must not linger in the repository.
	legacyKubernetesArchiveGlob = "kubernetes-*.tar.gz"
	requestTimeout              = 10 * time.Minute
)

type options struct {
	checkUpstream    bool
	crdsRef          string
	crdsRepo         string
	force            bool
	kubeVersion      string
	kubeVersionCount int
	kubernetesRef    string
	kubernetesRepo   string
	maxBundlesSize   string
	outputDir        string
	parallelism      int
	verify           bool
}

type schemaFile struct {
	content []byte
	name    string
}

func run(ctx context.Context) error {
	opts := parseFlags()

	if opts.verify {
		return verifyBundles(ctx, opts)
	}

	if opts.checkUpstream {
		return checkUpstream(ctx, &http.Client{Timeout: requestTimeout}, opts)
	}

	// Zero would deadlock the download semaphore and a negative value would panic building it.
	if opts.parallelism < 1 {
		return fmt.Errorf("parallelism must be positive, got %d", opts.parallelism)
	}

	existingIndex := readExistingIndex(filepath.Join(opts.outputDir, indexFileName))
	client := &http.Client{Timeout: requestTimeout}

	var index schemas.Index

	kubernetesBundle, err := ensureKubernetesBundle(ctx, client, opts, existingIndex)
	if err != nil {
		return fmt.Errorf("generate Kubernetes schemas bundle: %w", err)
	}

	index.Kubernetes = kubernetesBundle

	crdsBundle, err := ensureCRDsBundle(ctx, client, opts, existingIndex)
	if err != nil {
		return fmt.Errorf("generate CRDs schemas bundle: %w", err)
	}

	index.CRDs = crdsBundle

	if err := writeIndex(filepath.Join(opts.outputDir, indexFileName), index); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	if err := removeLegacyKubernetesBundles(ctx, opts.outputDir); err != nil {
		return fmt.Errorf("remove legacy bundles: %w", err)
	}

	// Checked once both archives are on disk and the index describes them, so that a run failing here
	// leaves the output directory self consistent and -verify reports the same thing.
	return checkBundlesSize(ctx, opts)
}

// ensureKubernetesBundle generates the bundle holding every collected minor version, skipping the work
// if the archive on disk is the one asked for already.
func ensureKubernetesBundle(ctx context.Context, client *http.Client, opts options, existingIndex *schemas.Index) (*schemas.Bundle, error) {
	minors, err := kubeMinors(opts.kubeVersion, opts.kubeVersionCount)
	if err != nil {
		return nil, err
	}

	archivePath := filepath.Join(opts.outputDir, kubernetesArchiveFileName)

	var existing *schemas.Bundle
	if existingIndex != nil {
		existing = existingIndex.Kubernetes
	}

	if !opts.force && isBundleUpToDate(archivePath, existing, opts.kubernetesRepo, opts.kubernetesRef) &&
		kubeVersionsMatch(existing.KubeVersions, opts.kubeVersion, minors) {
		log.Default.Info(ctx, "Embedded schemas for Kubernetes %s are up to date (%d schemas)",
			strings.Join(existing.KubeVersions, ", "), existing.FilesCount)

		return existing, nil
	}

	commit, err := resolveCommit(ctx, client, opts.kubernetesRepo, opts.kubernetesRef)
	if err != nil {
		return nil, fmt.Errorf("resolve %s of %s: %w", opts.kubernetesRef, opts.kubernetesRepo, err)
	}

	// The listing is what decides which patch of every older minor is collected, so it is fetched
	// before the versions are known rather than inside the downloader.
	dirSHAs, err := kubernetesVersionDirSHAs(ctx, client, opts.kubernetesRepo, commit)
	if err != nil {
		return nil, fmt.Errorf("list directories of %s: %w", opts.kubernetesRepo, err)
	}

	kubeVersions, err := resolveKubeVersions(dirSHAs, opts.kubeVersion, minors)
	if err != nil {
		return nil, err
	}

	files, err := downloadMergedKubernetesSchemas(ctx, client, opts, kubeVersions, dirSHAs, commit)
	if err != nil {
		return nil, err
	}

	trees := make(map[string]string, len(kubeVersions))

	for _, kubeVersion := range kubeVersions {
		trees[kubeVersion] = dirSHAs[kubernetesVersionDirName(kubeVersion)]
	}

	return packBundle(ctx, archivePath, files, schemas.Bundle{
		KubeVersions:   kubeVersions,
		UpstreamCommit: commit,
		UpstreamRef:    opts.kubernetesRef,
		UpstreamRepo:   opts.kubernetesRepo,
		UpstreamTrees:  trees,
	})
}

// checkUpstream reports whether the committed archives still match what upstream serves, without
// downloading a single schema: two requests, one per repository.
//
// The CRD catalog is taken whole, so its commit moving means its contents moved with it. The
// Kubernetes repository is different: it holds every version ever released, and the directories we
// take are frozen historical snapshots, so its ref moves all the time without changing anything we
// consume. Comparing the tree sha of each collected version tells the two apart.
//
// This is deliberately not part of generating or verifying: a build that followed upstream on its own
// would stop being reproducible and would need the network. It belongs on a schedule instead.
func checkUpstream(ctx context.Context, client *http.Client, opts options) error {
	indexPath := filepath.Join(opts.outputDir, indexFileName)

	index := readExistingIndex(indexPath)
	if index == nil || index.CRDs == nil || index.Kubernetes == nil {
		return fmt.Errorf("%s is missing or incomplete, run: task generate:validation-schemas", indexPath)
	}

	var stale []string

	crdsCommit, err := resolveCommit(ctx, client, index.CRDs.UpstreamRepo, index.CRDs.UpstreamRef)
	if err != nil {
		return fmt.Errorf("resolve %s of %s: %w", index.CRDs.UpstreamRef, index.CRDs.UpstreamRepo, err)
	}

	if crdsCommit != index.CRDs.UpstreamCommit {
		stale = append(stale, fmt.Sprintf("the CRD catalog moved from %s to %s", index.CRDs.UpstreamCommit[:7], crdsCommit[:7]))
	}

	dirSHAs, err := kubernetesVersionDirSHAs(ctx, client, index.Kubernetes.UpstreamRepo, index.Kubernetes.UpstreamRef)
	if err != nil {
		return fmt.Errorf("list directories of %s: %w", index.Kubernetes.UpstreamRepo, err)
	}

	for i, kubeVersion := range index.Kubernetes.KubeVersions {
		recorded, ok := index.Kubernetes.UpstreamTrees[kubeVersion]
		if !ok {
			stale = append(stale, fmt.Sprintf("Kubernetes %s records no upstream tree to compare", kubeVersion))

			continue
		}

		if current := dirSHAs[kubernetesVersionDirName(kubeVersion)]; current != recorded {
			stale = append(stale, fmt.Sprintf("the schemas of Kubernetes %s changed upstream", kubeVersion))
		}

		// The newest version is pinned in the Taskfile, so a newer patch of it is a bump to decide on,
		// not staleness. The older minors are collected at whatever patch upstream had at generation
		// time, so a newer one there means the archives no longer hold what they are supposed to.
		if i == 0 {
			continue
		}

		latest, err := latestPatchOfMinor(dirSHAs, minorsOf([]string{kubeVersion})[0])
		if err != nil {
			return err
		}

		if latest != kubeVersion {
			stale = append(stale, fmt.Sprintf("Kubernetes %s is collected, but upstream has %s", kubeVersion, latest))
		}
	}

	// A newer minor version showing up is not staleness: nothing we ship is out of date, there is just
	// a bump available. Reported, but not failed on.
	if newest := newestKubernetesVersionDir(dirSHAs); newest != "" && newest != index.Kubernetes.KubeVersions[0] {
		log.Default.Info(ctx, "Kubernetes %s is available upstream, pinned is %s", newest, index.Kubernetes.KubeVersions[0])
	}

	if len(stale) > 0 {
		return fmt.Errorf("embedded schemas are behind upstream: %s, run: task generate:validation-schemas:force",
			strings.Join(stale, "; "))
	}

	log.Default.Info(ctx, "Embedded schemas match upstream: %s@%s, and Kubernetes %s",
		index.CRDs.UpstreamRepo, crdsCommit[:7], strings.Join(index.Kubernetes.KubeVersions, ", "))

	return nil
}

// resolveKubeVersions turns the minor window into the exact versions to collect: the newest exactly as
// pinned, since that is the one nelm validates against, and every older minor at the highest patch
// upstream has, so that schema fixes released after ".0" are not missed.
func resolveKubeVersions(dirSHAs map[string]string, newest string, minors []string) ([]string, error) {
	newest = strings.TrimPrefix(newest, "v")

	if _, ok := dirSHAs[kubernetesVersionDirName(newest)]; !ok {
		return nil, fmt.Errorf("upstream has no %s directory", kubernetesVersionDirName(newest))
	}

	versions := make([]string, 0, len(minors))
	versions = append(versions, newest)

	for _, minor := range minors[1:] {
		latest, err := latestPatchOfMinor(dirSHAs, minor)
		if err != nil {
			return nil, err
		}

		versions = append(versions, latest)
	}

	return versions, nil
}

// verifyBundles checks the committed archives offline: the index lists exactly the versions asked
// for, every archive it references is present with the recorded digest, and nothing lingers besides.
// It is what keeps a bumped Kubernetes version or a hand-edited index from reaching a release.
func verifyBundles(ctx context.Context, opts options) error {
	indexPath := filepath.Join(opts.outputDir, indexFileName)

	index := readExistingIndex(indexPath)
	if index == nil {
		return fmt.Errorf("%s is missing or unreadable, run: task generate:validation-schemas", indexPath)
	}

	if index.CRDs == nil {
		return fmt.Errorf("%s lists no CRDs bundle, run: task generate:validation-schemas", indexPath)
	}

	if index.Kubernetes == nil {
		return fmt.Errorf("%s lists no Kubernetes bundle, run: task generate:validation-schemas", indexPath)
	}

	// Which patch of every older minor was collected is decided from the upstream listing at generation
	// time, so it is taken from the index as recorded. What is checked against the Taskfile is the pin
	// itself: the newest version and the window of minors around it.
	wantMinors, err := kubeMinors(opts.kubeVersion, opts.kubeVersionCount)
	if err != nil {
		return err
	}

	gotVersions := index.Kubernetes.KubeVersions

	if !kubeVersionsMatch(gotVersions, opts.kubeVersion, wantMinors) {
		return fmt.Errorf("%s was generated from Kubernetes versions %s, which is not the pinned %s over %d minors, run: task generate:validation-schemas",
			indexPath, strings.Join(gotVersions, ", "), opts.kubeVersion, opts.kubeVersionCount)
	}

	for _, kubeVersion := range gotVersions {
		if index.Kubernetes.UpstreamTrees[kubeVersion] == "" {
			return fmt.Errorf("%s records no upstream tree for Kubernetes %s, run: task generate:validation-schemas:force",
				indexPath, kubeVersion)
		}
	}

	for _, want := range []struct {
		bundle *schemas.Bundle
		name   string
		ref    string
	}{
		{bundle: index.Kubernetes, name: "kubernetes", ref: opts.kubernetesRef},
		{bundle: index.CRDs, name: "crds", ref: opts.crdsRef},
	} {
		if want.bundle.UpstreamRef != want.ref {
			return fmt.Errorf("%s records the %s bundle as generated from ref %q, but %q is asked for, run: task generate:validation-schemas",
				indexPath, want.name, want.bundle.UpstreamRef, want.ref)
		}
	}

	archives := map[string]*schemas.Bundle{
		crdsArchiveFileName:       index.CRDs,
		kubernetesArchiveFileName: index.Kubernetes,
	}

	for name, bundle := range archives {
		path := filepath.Join(opts.outputDir, name)

		archive, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w, run: task generate:validation-schemas", path, err)
		}

		digest := sha256.Sum256(archive)

		if recorded := hex.EncodeToString(digest[:]); recorded != bundle.SHA256 {
			return fmt.Errorf("%s has digest %s, but %s records %s, run: task generate:validation-schemas:force",
				path, recorded, indexPath, bundle.SHA256)
		}
	}

	paths, err := legacyKubernetesArchives(opts.outputDir)
	if err != nil {
		return err
	}

	if len(paths) > 0 {
		return fmt.Errorf("%s is not embedded any more, run: task generate:validation-schemas", paths[0])
	}

	if err := checkBundlesSize(ctx, opts); err != nil {
		return err
	}

	log.Default.Info(ctx, "Embedded schemas are consistent: %d Kubernetes schemas of %s, and %d CRD schemas",
		index.Kubernetes.FilesCount, strings.Join(index.Kubernetes.KubeVersions, ", "), index.CRDs.FilesCount)

	return nil
}

// kubeVersionsMatch reports whether the recorded versions are the ones asked for: the newest exactly as
// pinned, and one version per minor of the window. Which patch each older minor sits at is not part of
// the question, since that is decided from the upstream listing and cannot be checked offline.
func kubeVersionsMatch(recorded []string, newest string, minors []string) bool {
	if len(recorded) == 0 || recorded[0] != strings.TrimPrefix(newest, "v") {
		return false
	}

	return slices.Equal(minorsOf(recorded), minors)
}

// latestPatchOfMinor picks the highest patch release of a minor that upstream has a schema directory
// for.
func latestPatchOfMinor(dirSHAs map[string]string, minor string) (string, error) {
	var (
		latest      string
		latestPatch = -1
	)

	for dirName := range dirSHAs {
		major, dirMinor, patch, err := parseKubernetesVersionDirName(dirName)
		if err != nil {
			continue
		}

		if fmt.Sprintf("%d.%d", major, dirMinor) != minor || patch <= latestPatch {
			continue
		}

		latest, latestPatch = fmt.Sprintf("%d.%d.%d", major, dirMinor, patch), patch
	}

	if latest == "" {
		return "", fmt.Errorf("upstream has no schema directory for Kubernetes %s", minor)
	}

	return latest, nil
}

// newestKubernetesVersionDir picks the highest "v<major>.<minor>.<patch>-standalone" directory name
// upstream has, so that an available version bump can be pointed out. Ordered on all three numbers, so
// that the answer does not depend on map iteration order among patches of the same minor.
func newestKubernetesVersionDir(dirSHAs map[string]string) string {
	var (
		newest       string
		newestWeight int
	)

	for dirName := range dirSHAs {
		major, minor, patch, err := parseKubernetesVersionDirName(dirName)
		if err != nil {
			continue
		}

		if weight := major*1_000_000 + minor*1_000 + patch; weight > newestWeight {
			newest, newestWeight = fmt.Sprintf("%d.%d.%d", major, minor, patch), weight
		}
	}

	return newest
}

func ensureCRDsBundle(ctx context.Context, client *http.Client, opts options, existingIndex *schemas.Index) (*schemas.Bundle, error) {
	archivePath := filepath.Join(opts.outputDir, crdsArchiveFileName)

	var existing *schemas.Bundle
	if existingIndex != nil {
		existing = existingIndex.CRDs
	}

	if !opts.force && isBundleUpToDate(archivePath, existing, opts.crdsRepo, opts.crdsRef) {
		log.Default.Info(ctx, "Embedded CRD schemas are up to date (%d schemas)", existing.FilesCount)

		return existing, nil
	}

	commit, err := resolveCommit(ctx, client, opts.crdsRepo, opts.crdsRef)
	if err != nil {
		return nil, fmt.Errorf("resolve %s of %s: %w", opts.crdsRef, opts.crdsRepo, err)
	}

	log.Default.Info(ctx, "Downloading CRD schemas from %s@%s", opts.crdsRepo, commit[:7])

	// The catalog is thousands of files, so it is fetched as one repository tarball rather than
	// file by file.
	files, err := downloadCRDsCatalog(ctx, client, opts.crdsRepo, commit)
	if err != nil {
		return nil, fmt.Errorf("download catalog: %w", err)
	}

	return packBundle(ctx, archivePath, files, schemas.Bundle{
		UpstreamCommit: commit,
		UpstreamRef:    opts.crdsRef,
		UpstreamRepo:   opts.crdsRepo,
	})
}

// kubeMinors lists the "<major>.<minor>" windows to collect schemas from, newest first: the one the
// asked for version belongs to, and the minors right before it. Computed without the network, which is
// what lets the up to date check and -verify work offline; which patch of each minor is actually
// collected is decided from the upstream listing, see resolveKubeVersions.
func kubeMinors(newest string, count int) ([]string, error) {
	if newest == "" {
		return nil, errors.New("no Kubernetes version given, run: task generate:validation-schemas")
	}

	if count < 1 {
		return nil, fmt.Errorf("kube version count must be positive, got %d", count)
	}

	major, minor, _, err := parseKubeVersion(newest)
	if err != nil {
		return nil, err
	}

	minors := make([]string, 0, count)

	for i := 0; i < count && minor-i >= 0; i++ {
		minors = append(minors, fmt.Sprintf("%d.%d", major, minor-i))
	}

	return minors, nil
}

// minorsOf reduces exact versions to their "<major>.<minor>" parts, keeping the order.
func minorsOf(kubeVersions []string) []string {
	minors := make([]string, 0, len(kubeVersions))

	for _, kubeVersion := range kubeVersions {
		major, minor, _, err := parseKubeVersion(kubeVersion)
		if err != nil {
			minors = append(minors, kubeVersion)

			continue
		}

		minors = append(minors, fmt.Sprintf("%d.%d", major, minor))
	}

	return minors
}

func parseKubernetesVersionDirName(dirName string) (major, minor, patch int, err error) {
	kubeVersion, ok := strings.CutSuffix(dirName, "-standalone")
	if !ok {
		return 0, 0, 0, fmt.Errorf("%q is not a schema directory", dirName)
	}

	return parseKubeVersion(kubeVersion)
}

// removeLegacyKubernetesBundles drops the per version archives that the merged Kubernetes bundle
// replaced, so that a tree generated by an older schemagen does not keep carrying them.
func removeLegacyKubernetesBundles(ctx context.Context, outputDir string) error {
	paths, err := legacyKubernetesArchives(outputDir)
	if err != nil {
		return err
	}

	for _, path := range paths {
		log.Default.Info(ctx, "Removing no longer embedded %s", path)

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	return nil
}

// checkBundlesSize fails when the archives outgrow the budget. They are embedded verbatim, so their
// size is what every nelm binary and every module importing nelm pays, whether the schemas are used or
// not.
func checkBundlesSize(ctx context.Context, opts options) error {
	limit, err := humanize.ParseBytes(opts.maxBundlesSize)
	if err != nil {
		return fmt.Errorf("invalid max bundles size %q: %w", opts.maxBundlesSize, err)
	}

	if limit == 0 {
		return fmt.Errorf("max bundles size must be positive, got %q", opts.maxBundlesSize)
	}

	var total uint64

	for _, name := range []string{kubernetesArchiveFileName, crdsArchiveFileName} {
		stat, err := os.Stat(filepath.Join(opts.outputDir, name))
		if err != nil {
			return fmt.Errorf("stat %s: %w", filepath.Join(opts.outputDir, name), err)
		}

		total += uint64(stat.Size())
	}

	if total > limit {
		return fmt.Errorf("the embedded archives take %s, over the %s limit: collect fewer Kubernetes minor versions, or raise -max-bundles-size if the budget is meant to grow",
			humanize.IBytes(total), humanize.IBytes(limit))
	}

	log.Default.Info(ctx, "Embedded archives take %s of the %s limit", humanize.IBytes(total), humanize.IBytes(limit))

	return nil
}

// isBundleUpToDate reports whether the archive on disk was already generated from the requested
// repository and ref, so that repeated builds do not hit the network. It cannot tell that the ref
// itself has moved, nor that a new patch release appeared: that is what -check-upstream is for.
func isBundleUpToDate(archivePath string, existing *schemas.Bundle, repo, ref string) bool {
	if existing == nil || existing.UpstreamRepo != repo || existing.UpstreamRef != ref {
		return false
	}

	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return false
	}

	digest := sha256.Sum256(archive)

	return hex.EncodeToString(digest[:]) == existing.SHA256
}

func legacyKubernetesArchives(outputDir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(outputDir, legacyKubernetesArchiveGlob))
	if err != nil {
		return nil, fmt.Errorf("list bundles in %s: %w", outputDir, err)
	}

	return paths, nil
}

// packBundle strips, packs and persists the schemas, returning the index entry describing them.
func packBundle(ctx context.Context, archivePath string, files []schemaFile, bundle schemas.Bundle) (*schemas.Bundle, error) {
	archive, uncompressedSize, err := buildArchive(files)
	if err != nil {
		return nil, fmt.Errorf("build archive: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(archivePath), err)
	}

	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", archivePath, err)
	}

	digest := sha256.Sum256(archive)

	bundle.FilesCount = len(files)
	bundle.SHA256 = hex.EncodeToString(digest[:])
	bundle.UncompressedSize = uncompressedSize

	log.Default.Info(ctx, "Packed %d schemas into %s (%.0f KiB compressed, %.1f MiB unpacked)",
		bundle.FilesCount, archivePath, float64(len(archive))/1024, float64(uncompressedSize)/1024/1024)

	return &bundle, nil
}

func parseFlags() options {
	var opts options

	flag.StringVar(&opts.crdsRef, "crds-ref", defaultCRDsRef, "Git ref of the CRDs catalog to download from")
	flag.StringVar(&opts.crdsRepo, "crds-repo", defaultCRDsRepo, "GitHub repository to download the CRD schemas from")
	flag.BoolVar(&opts.force, "force", false, "Regenerate the bundles even if they are already up to date")
	flag.StringVar(&opts.kubeVersion, "kube-version", "",
		"Newest Kubernetes version to collect schemas from, which is also the version nelm validates against (required)")
	flag.IntVar(&opts.kubeVersionCount, "kube-version-count", defaultKubeVersionCount,
		"How many Kubernetes minor versions to collect schemas from, counting back from -kube-version. Every one but the newest is taken at the latest patch upstream has")
	flag.StringVar(&opts.kubernetesRef, "kubernetes-ref", defaultKubernetesRef, "Git ref of the Kubernetes schemas repository to download from")
	flag.StringVar(&opts.kubernetesRepo, "kubernetes-repo", defaultKubernetesRepo, "GitHub repository to download the Kubernetes schemas from")
	flag.StringVar(&opts.maxBundlesSize, "max-bundles-size", defaultMaxBundlesSize,
		"Fail when the archives add up to more than this, since all of it ends up in every nelm binary. Accepts 5MiB, 5MB or plain bytes")
	flag.StringVar(&opts.outputDir, "output", defaultOutputDir, "Directory to write the bundles and their index to")
	flag.IntVar(&opts.parallelism, "parallelism", defaultParallelism, "How many schemas to download concurrently")
	flag.BoolVar(&opts.verify, "verify", false, "Check the committed archives against their index and exit, without downloading anything")
	flag.BoolVar(&opts.checkUpstream, "check-upstream", false, "Report whether upstream has schemas the committed archives do not, and exit, without downloading them")
	flag.Parse()

	return opts
}

func parseKubeVersion(kubeVersion string) (major, minor, patch int, err error) {
	parts := strings.Split(strings.TrimPrefix(kubeVersion, "v"), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid kube version %q", kubeVersion)
	}

	numbers := make([]int, 0, len(parts))

	for _, part := range parts {
		number, convErr := strconv.Atoi(part)
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("invalid kube version %q: %w", kubeVersion, convErr)
		}

		numbers = append(numbers, number)
	}

	return numbers[0], numbers[1], numbers[2], nil
}

func readExistingIndex(path string) *schemas.Index {
	indexBytes, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var index schemas.Index

	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return nil
	}

	return &index
}

func writeIndex(path string, index schemas.Index) error {
	indexBytes, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode index: %w", err)
	}

	if err := os.WriteFile(path, append(indexBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		log.Default.Error(ctx, "Error: %s", err)
		os.Exit(1)
	}
}
