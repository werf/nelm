package action

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/samber/lo"
	yamlv3 "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/chartyaml"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/featgate"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	v3util "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3/util"
	"github.com/werf/nelm/v2/pkg/helm/intern/third_party/dep/fs"
	"github.com/werf/nelm/v2/pkg/helm/intern/tlsutil"
	helmaction "github.com/werf/nelm/v2/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	v2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/cli/values"
	helmdownloader "github.com/werf/nelm/v2/pkg/helm/pkg/downloader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/helm/pkg/strvals"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/resource/spec"
	"github.com/werf/nelm/v2/pkg/ts"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const (
	legacyExtraAnnotationsFileName = "extra_annotations.json"
	legacyExtraLabelsFileName      = "extra_labels.json"
)

var yamlErrorLineRegexp = regexp.MustCompile(`line (\d+)`)

type ChartPackOptions struct {
	common.ChartRepoConnectionOptions

	// AppVersion overrides appVersion in Chart.yaml.
	AppVersion string
	// ChartRepoSkipUpdate, when true, skips updating the chart repositories index before
	// downloading dependencies.
	ChartRepoSkipUpdate bool
	// ContentCache is the path to the cache of downloaded dependency charts.
	// Defaults to the Helm content cache.
	ContentCache string
	// DefaultPatchesDisable, when not nil, replaces the stored switch that makes the deployment
	// ignore the patches.yaml files of the chart and its subcharts.
	DefaultPatchesDisable *bool
	// DefaultSecretValuesDisable, when not nil, replaces the stored switch that makes the
	// deployment ignore the secret-values.yaml file of the top-level chart.
	DefaultSecretValuesDisable *bool
	// DefaultValuesDisable, when not nil, replaces the stored switch that makes the deployment
	// ignore the values.yaml file of the top-level chart.
	DefaultValuesDisable *bool
	// DenoBinaryPath, if specified, uses this path as the Deno binary instead of auto-downloading.
	DenoBinaryPath string
	// DependencyUpdate, when true, updates Chart.lock and charts/ of the chart directory from
	// Chart.yaml before packing.
	DependencyUpdate bool
	// Destination is the directory to write the archive to. Defaults to the current directory.
	Destination string
	// ExtraAnnotations are merged per key over the stored ones.
	ExtraAnnotations map[string]string
	// ExtraLabels are merged per key over the stored ones.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are merged per key over the stored ones.
	ExtraRuntimeAnnotations map[string]string
	// ExtraValues, when not nil, replace the stored extra values. An empty map removes them.
	ExtraValues map[string]interface{}
	// Images, when not nil, replace the helm.sh/images entries that previous packs added to
	// Chart.yaml. An empty slice removes them. Entries written by the chart author are kept.
	Images []chartyaml.Image
	// Name overrides the chart name.
	Name string
	// PatchesFiles are appended to the stored patches files.
	PatchesFiles []string
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials,
	// used to download dependencies from OCI registries.
	// Defaults to DefaultDockerConfig/config.json.
	RegistryCredentialsPath string
	// Release, when not nil, replaces the stored release defaults.
	Release *werfdata.ReleaseMetadata
	// ReleaseInfoAnnotations are merged per key over the stored ones.
	ReleaseInfoAnnotations map[string]string
	// RepositoryCache is the path to the cache of chart repository indexes.
	// Defaults to the Helm repository cache.
	RepositoryCache string
	// RepositoryConfig is the path to the file with chart repository names and URLs.
	// Defaults to the Helm repositories.yaml.
	RepositoryConfig string
	// ResetValues, when true, drops everything stored in .werf-data and the helm.sh/images
	// entries previous packs added before applying the inputs of this pack.
	ResetValues bool
	// RootSetJSON are appended to the stored --set-root-json values.
	RootSetJSON []string
	// SecretValuesFiles are werf-encrypted values files, merged in order over the stored secret
	// values without decrypting them. Every value must be encrypted.
	SecretValuesFiles []string
	// Sign, when true, signs the archive with SignKey from SignKeyring and writes the
	// provenance file next to it.
	Sign bool
	// SignKey is the name of the key to sign with.
	SignKey string
	// SignKeyring is the path to the keyring with the signing key.
	SignKeyring string
	// SignPassphraseFile is the path to a file with the passphrase of the signing key, "-" for
	// stdin. Prompted for if not set.
	SignPassphraseFile string
	// TSNoRebuild, when true, reuses the existing TypeScript bundle instead of rebuilding it.
	TSNoRebuild bool
	// TempDirPath is the directory for temporary files during the operation.
	// The system directory for temporary files is used if not specified.
	TempDirPath string
	// ValuesFiles are merged in order over the stored values.
	ValuesFiles []string
	// ValuesSet are appended to the stored --set values.
	ValuesSet []string
	// ValuesSetFile are read and appended to the stored --set-file values.
	ValuesSetFile []string
	// ValuesSetJSON are appended to the stored --set-json values.
	ValuesSetJSON []string
	// ValuesSetLiteral are appended to the stored --set-literal values.
	ValuesSetLiteral []string
	// ValuesSetString are appended to the stored --set-string values.
	ValuesSetString []string
	// Version overrides the chart version.
	Version string
}

type ChartPackResult struct {
	ArchivePath  string
	ChartName    string
	ChartVersion string
	// ProvenancePath is the path to the provenance file, empty if the archive is not signed.
	ProvenancePath string
}

// ChartPack packs the chart directory into an archive, storing the publish-time inputs from
// opts in .werf-data of the chart on top of the ones the directory already has.
func ChartPack(ctx context.Context, chartPath string, opts ChartPackOptions) (*ChartPackResult, error) {
	opts, err := applyChartPackOptionsDefaults(opts)
	if err != nil {
		return nil, fmt.Errorf("build chart pack options: %w", err)
	}

	if opts.Sign && (opts.SignKey == "" || opts.SignKeyring == "") {
		return nil, errors.New("signing key name and keyring are required for signing")
	}

	chartPath, err = filepath.Abs(chartPath)
	if err != nil {
		return nil, fmt.Errorf("get absolute path of %q: %w", chartPath, err)
	}

	registryClient, err := newChartPackRegistryClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("construct registry client: %w", err)
	}

	if opts.DependencyUpdate {
		if err := newChartPackDependencyDownloader(ctx, chartPath, registryClient, opts).Update(ctx); err != nil {
			return nil, fmt.Errorf("update dependencies: %w", err)
		}
	}

	ctx = common.ContextWithHelmOptions(ctx, common.HelmOptions{
		ChartLoadOpts: common.ChartLoadOptions{
			ChartDepsDownloader:    newChartPackDependencyDownloader(ctx, chartPath, registryClient, opts),
			ChartType:              common.LegacyChartTypeChart,
			DefaultChartAPIVersion: v2chart.APIVersionV2,
			DefaultChartName:       opts.Name,
			DefaultChartVersion:    opts.Version,
			NoSecrets:              true,
		},
	})

	loadedChart, err := loader.LoadDir(ctx, chartPath)
	if err != nil {
		return nil, fmt.Errorf("load chart %q: %w", chartPath, err)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	if err := checkDependenciesWerfData(chartAccessor); err != nil {
		return nil, err
	}

	if featgate.FeatGateTypescript.Enabled() {
		if err := ts.BundleChartsRecursive(ctx, chartAccessor, chartPath, !opts.TSNoRebuild, opts.DenoBinaryPath); err != nil {
			return nil, fmt.Errorf("process TypeScript files in chart: %w", err)
		}
	}

	if deps := chartAccessor.MetaDependencies(); len(deps) > 0 {
		if err := helmaction.CheckDependencies(loadedChart, deps); err != nil {
			return nil, fmt.Errorf("check dependencies: %w", err)
		}
	}

	werfData, ownedImages, chartFiles, err := buildChartPackWerfData(ctx, chartAccessor, opts)
	if err != nil {
		return nil, err
	}

	werfDataFiles, err := werfData.ChartFiles()
	if err != nil {
		return nil, fmt.Errorf("build %s files: %w", werfdata.DirName, err)
	}

	if len(chartAccessor.WerfDataFiles()) == 0 {
		emptyWerfDataFiles, err := (&werfdata.Data{}).ChartFiles()
		if err != nil {
			return nil, fmt.Errorf("build empty %s files: %w", werfdata.DirName, err)
		}

		if reflect.DeepEqual(werfDataFiles, emptyWerfDataFiles) {
			werfDataFiles = nil
		}
	}

	for _, f := range werfDataFiles {
		if int64(len(f.Data)) > archive.MaxDecompressedFileSize {
			return nil, fmt.Errorf("%s is %d bytes, more than the %d bytes a chart loader accepts for a single file", f.Name, len(f.Data), archive.MaxDecompressedFileSize)
		}
	}

	chartYAMLEdit := chartyaml.EditOptions{
		AppVersion: opts.AppVersion,
		Name:       opts.Name,
		Version:    opts.Version,
	}

	if opts.Images != nil || opts.ResetValues {
		currentImagesAnnotation := chartAnnotations(loadedChart)[chartyaml.ImagesAnnotation]

		imagesAnnotation, err := chartyaml.UpdateImages(ctx, currentImagesAnnotation, ownedImages, opts.Images)
		if err != nil {
			return nil, fmt.Errorf("update %s annotation: %w", chartyaml.ImagesAnnotation, err)
		}

		switch imagesAnnotation {
		case currentImagesAnnotation:
		case "":
			chartYAMLEdit.DeleteAnnotations = []string{chartyaml.ImagesAnnotation}
		default:
			chartYAMLEdit.SetAnnotations = map[string]string{chartyaml.ImagesAnnotation: imagesAnnotation}
		}
	}

	// The archive and its provenance file are built in a temporary directory and moved into the
	// destination only when everything succeeded, so that a failed pack never leaves a signature
	// next to an archive it doesn't sign.
	stagingDir, err := os.MkdirTemp(opts.TempDirPath, "chart-pack-")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	stagedArchivePath, err := saveChartPackArchive(loadedChart, chartFiles, werfDataFiles, chartYAMLEdit, stagingDir)
	if err != nil {
		return nil, fmt.Errorf("save chart archive: %w", err)
	}

	if err := rewriteChartPackArchiveMetadata(ctx, stagedArchivePath, chartAccessor, chartYAMLEdit); err != nil {
		return nil, fmt.Errorf("write Chart.yaml and Chart.lock into the archive: %w", err)
	}

	if err := checkChartPackArchiveSize(stagedArchivePath); err != nil {
		return nil, err
	}

	archivePath := filepath.Join(opts.Destination, filepath.Base(stagedArchivePath))
	result := &ChartPackResult{
		ArchivePath:  archivePath,
		ChartName:    chartAccessor.Name(),
		ChartVersion: chartVersion(loadedChart),
	}

	if opts.Sign {
		pkg := &helmaction.Package{
			Key:            opts.SignKey,
			Keyring:        opts.SignKeyring,
			PassphraseFile: opts.SignPassphraseFile,
		}

		if err := pkg.Clearsign(stagedArchivePath); err != nil {
			return nil, fmt.Errorf("sign %q: %w", archivePath, err)
		}

		result.ProvenancePath = archivePath + ".prov"
	}

	if err := os.MkdirAll(opts.Destination, 0o755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}

	// The old provenance file goes first, so that no failure below leaves it next to an archive it
	// doesn't sign.
	if err := os.Remove(archivePath + ".prov"); err == nil {
		if !opts.Sign {
			log.Default.Warn(ctx, "Removed %q: it signed a previous archive and the new one is not signed", archivePath+".prov")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove provenance file of a previous archive: %w", err)
	}

	if err := replaceFile(stagedArchivePath, archivePath); err != nil {
		return nil, fmt.Errorf("move archive into place: %w", err)
	}

	if opts.Sign {
		if err := replaceFile(stagedArchivePath+".prov", result.ProvenancePath); err != nil {
			return nil, fmt.Errorf("move provenance file into place: %w", err)
		}
	}

	log.Default.Info(ctx, "Successfully packaged chart and saved it to: %s", archivePath)

	return result, nil
}

// buildChartPackWerfData returns the .werf-data to pack, the helm.sh/images entries the
// previous packs added and the chart files without the ones moved into .werf-data.
func buildChartPackWerfData(ctx context.Context, chartAccessor helmchart.Accessor, opts ChartPackOptions) (*werfdata.Data, []string, []*chartcommon.File, error) {
	data := &werfdata.Data{Files: map[string][]byte{}}
	if len(chartAccessor.WerfDataFiles()) > 0 {
		var err error

		data, err = werfdata.Parse(chartAccessor.WerfDataFiles())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
		}
	}

	if data.SecretValues != nil && !opts.ResetValues {
		if _, err := parseEncryptedValues(data.SecretValues); err != nil {
			return nil, nil, nil, fmt.Errorf("%s/%s: %w", werfdata.DirName, werfdata.SecretValuesFileName, err)
		}
	}

	ownedImages := data.Metadata.Images

	chartFiles, err := migrateLegacyExtraAnnotationsAndLabels(chartAccessor.Files(), data)
	if err != nil {
		return nil, nil, nil, err
	}

	if opts.ResetValues {
		data = &werfdata.Data{Files: map[string][]byte{}}
	}

	if err := addChartPackInputs(ctx, data, opts); err != nil {
		return nil, nil, nil, err
	}

	return data, ownedImages, chartFiles, nil
}

func addChartPackInputs(ctx context.Context, data *werfdata.Data, opts ChartPackOptions) error {
	for _, valuesFile := range opts.ValuesFiles {
		vals, err := (&values.Options{ValueFiles: []string{valuesFile}}).MergeValues(ctx, getter.Getters())
		if err != nil {
			return fmt.Errorf("read values file %q: %w", valuesFile, err)
		}

		data.Values = chartcommon.MergeMaps(lo.Ternary(data.Values != nil, data.Values, map[string]any{}), vals)
	}

	if len(opts.SecretValuesFiles) > 0 {
		if err := mergeChartPackSecretValues(ctx, data, opts.SecretValuesFiles); err != nil {
			return err
		}
	}

	setValuesOpts := &values.Options{
		Values:        opts.ValuesSet,
		StringValues:  opts.ValuesSetString,
		JSONValues:    opts.ValuesSetJSON,
		LiteralValues: opts.ValuesSetLiteral,
	}
	if _, err := setValuesOpts.MergeSetValues(ctx, getter.Getters(), map[string]any{}, values.MergeSetValuesOptions{}); err != nil {
		return fmt.Errorf("parse set values: %w", err)
	}

	for _, rootSetJSON := range opts.RootSetJSON {
		if err := strvals.ParseJSON(rootSetJSON, map[string]any{}); err != nil {
			return fmt.Errorf("parse root set JSON value %q: %w", rootSetJSON, err)
		}
	}

	data.Metadata.Values.ValuesSet = append(data.Metadata.Values.ValuesSet, opts.ValuesSet...)
	data.Metadata.Values.ValuesSetString = append(data.Metadata.Values.ValuesSetString, opts.ValuesSetString...)
	data.Metadata.Values.ValuesSetJSON = append(data.Metadata.Values.ValuesSetJSON, opts.ValuesSetJSON...)
	data.Metadata.Values.ValuesSetLiteral = append(data.Metadata.Values.ValuesSetLiteral, opts.ValuesSetLiteral...)
	data.Metadata.Values.ValuesSetRootJSON = append(data.Metadata.Values.ValuesSetRootJSON, opts.RootSetJSON...)

	for _, value := range opts.ValuesSetFile {
		contents := map[string]string{}
		if _, err := (&values.Options{FileValues: []string{value}}).MergeSetValues(ctx, getter.Getters(), map[string]any{}, values.MergeSetValuesOptions{SetFilesCache: contents}); err != nil {
			return fmt.Errorf("read --set-file %q: %w", value, err)
		}

		record := werfdata.SetFileRecord{Value: value, Contents: map[string]string{}}
		filePaths := lo.Keys(contents)
		sort.Strings(filePaths)

		for _, filePath := range filePaths {
			name := path.Join(werfdata.SetFilesDirName, strconv.Itoa(nextWerfDataFileIndex(data.Files, werfdata.SetFilesDirName)))
			data.Files[name] = []byte(contents[filePath])
			record.Contents[filePath] = name
		}

		data.Metadata.Values.ValuesSetFile = append(data.Metadata.Values.ValuesSetFile, record)
	}

	for _, patchesFile := range opts.PatchesFiles {
		patchesData, err := readChartPackInputFile(ctx, patchesFile)
		if err != nil {
			return fmt.Errorf("read patches file %q: %w", patchesFile, err)
		}

		if _, err := spec.ParsePatchesData(patchesData); err != nil {
			return fmt.Errorf("patches file %q: %w", patchesFile, err)
		}

		name := path.Join(werfdata.PatchesDirName, strconv.Itoa(nextWerfDataFileIndex(data.Files, werfdata.PatchesDirName)))
		data.Files[name] = patchesData
		data.Metadata.Patches = append(data.Metadata.Patches, name)
	}

	if opts.DefaultValuesDisable != nil {
		data.Metadata.Values.DefaultValuesDisable = *opts.DefaultValuesDisable
	}

	if opts.DefaultSecretValuesDisable != nil {
		data.Metadata.Values.DefaultSecretValuesDisable = *opts.DefaultSecretValuesDisable
	}

	if opts.DefaultPatchesDisable != nil {
		data.Metadata.DefaultPatchesDisable = *opts.DefaultPatchesDisable
	}

	if opts.Release != nil {
		data.Metadata.Release = lo.ToPtr(*opts.Release)
	}

	data.Metadata.ExtraAnnotations = mergeStringMaps(data.Metadata.ExtraAnnotations, opts.ExtraAnnotations)
	data.Metadata.ExtraLabels = mergeStringMaps(data.Metadata.ExtraLabels, opts.ExtraLabels)
	data.Metadata.ExtraRuntimeAnnotations = mergeStringMaps(data.Metadata.ExtraRuntimeAnnotations, opts.ExtraRuntimeAnnotations)
	data.Metadata.ReleaseInfoAnnotations = mergeStringMaps(data.Metadata.ReleaseInfoAnnotations, opts.ReleaseInfoAnnotations)

	if opts.ExtraValues != nil {
		data.ExtraValues = lo.Ternary(len(opts.ExtraValues) > 0, opts.ExtraValues, nil)
	}

	if opts.Images != nil {
		data.Metadata.Images = lo.Map(opts.Images, func(image chartyaml.Image, _ int) string {
			return image.Name
		})
	} else if opts.ResetValues {
		data.Metadata.Images = nil
	}

	return nil
}

// checkDependenciesWerfData fails if the .werf-data of a dependency, at any depth, is invalid or
// has an unsupported apiVersion.
func checkDependenciesWerfData(chartAccessor helmchart.Accessor) error {
	for _, dep := range chartAccessor.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return fmt.Errorf("create dependency accessor: %w", err)
		}

		if len(depAccessor.WerfDataFiles()) > 0 {
			depData, err := werfdata.Parse(depAccessor.WerfDataFiles())
			if err != nil {
				return fmt.Errorf("dependency %q: %w", depAccessor.Name(), err)
			}

			if depData.SecretValues != nil {
				if _, err := parseEncryptedValues(depData.SecretValues); err != nil {
					return fmt.Errorf("dependency %q: %s/%s: %w", depAccessor.Name(), werfdata.DirName, werfdata.SecretValuesFileName, err)
				}
			}
		}

		if err := checkDependenciesWerfData(depAccessor); err != nil {
			return fmt.Errorf("dependency %q: %w", depAccessor.Name(), err)
		}
	}

	return nil
}

func mergeChartPackSecretValues(ctx context.Context, data *werfdata.Data, secretValuesFiles []string) error {
	merged := map[string]any{}
	if data.SecretValues != nil {
		var err error
		if merged, err = parseEncryptedValues(data.SecretValues); err != nil {
			return fmt.Errorf("%s/%s: %w", werfdata.DirName, werfdata.SecretValuesFileName, err)
		}
	}

	for _, secretValuesFile := range secretValuesFiles {
		fileData, err := readChartPackInputFile(ctx, secretValuesFile)
		if err != nil {
			return fmt.Errorf("read secret values file %q: %w", secretValuesFile, err)
		}

		vals, err := parseEncryptedValues(fileData)
		if err != nil {
			return fmt.Errorf("secret values file %q: %w", secretValuesFile, err)
		}

		merged = chartcommon.MergeMaps(merged, vals)
	}

	secretValuesData, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal secret values: %w", err)
	}

	data.SecretValues = secretValuesData

	return nil
}

// parseEncryptedValues parses a secret values file, failing unless it is a single YAML document
// with every value encrypted. The checks run on the YAML nodes as written rather than on decoded
// values, so that nothing published can hide a plaintext value. Errors don't include the file
// contents.
func parseEncryptedValues(data []byte) (map[string]any, error) {
	var (
		root           *yamlv3.Node
		emptyDocuments int
	)

	decoder := yamlv3.NewDecoder(bytes.NewReader(data))
	for {
		var document yamlv3.Node
		if err := decoder.Decode(&document); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, sanitizeYAMLError(err)
		}

		if len(document.Content) == 0 || isNullYAMLScalar(document.Content[0]) {
			emptyDocuments++
			continue
		}

		// Only a document with values first and empty documents after it are accepted, since
		// the values are read from the first document.
		if root != nil || emptyDocuments > 0 {
			return nil, errors.New("has several YAML documents: remove the YAML document separators (---) so that only one document with values is left")
		}

		root = document.Content[0]
	}

	if root == nil {
		return map[string]any{}, nil
	}

	if root.Kind != yamlv3.MappingNode {
		return nil, fmt.Errorf("expected a mapping at the top level, got %s at line %d", yamlNodeKindName(root.Kind), root.Line)
	}

	if err := checkEncryptedYAMLNode(root, nil); err != nil {
		return nil, err
	}

	vals := map[string]any{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, sanitizeYAMLError(err)
	}

	return vals, nil
}

// checkEncryptedYAMLNode fails unless every non-null scalar value under node, wherever it is, is
// a value encrypted by werf. Mapping keys can't repeat and only standard tags are accepted on
// collections, since both change what the decoded values are.
func checkEncryptedYAMLNode(node *yamlv3.Node, keyPath []string) error {
	switch node.Kind {
	case yamlv3.MappingNode:
		if node.Tag != "!!map" {
			return fmt.Errorf("unsupported tag %q at line %d, column %d", node.Tag, node.Line, node.Column)
		}

		seen := map[string]bool{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yamlv3.ScalarNode {
				return fmt.Errorf("unsupported non-scalar key at line %d, column %d", key.Line, key.Column)
			}

			// A repeated merge key only adds merge sources, which are checked like other values.
			if key.Tag == "!!merge" {
				continue
			}

			if seen[key.Value] {
				return fmt.Errorf("duplicate key %q at line %d, column %d", key.Value, key.Line, key.Column)
			}

			seen[key.Value] = true
		}

		for i := 0; i+1 < len(node.Content); i += 2 {
			if err := checkEncryptedYAMLNode(node.Content[i+1], append(keyPath, node.Content[i].Value)); err != nil {
				return err
			}
		}
	case yamlv3.SequenceNode:
		if node.Tag != "!!seq" {
			return fmt.Errorf("unsupported tag %q at line %d, column %d", node.Tag, node.Line, node.Column)
		}

		for i, item := range node.Content {
			if err := checkEncryptedYAMLNode(item, append(keyPath, fmt.Sprintf("[%d]", i))); err != nil {
				return err
			}
		}
	case yamlv3.ScalarNode:
		if node.Style&yamlv3.TaggedStyle != 0 && node.Tag != "!!str" && node.Tag != "!!null" {
			return fmt.Errorf("unsupported tag %q at line %d, column %d", node.Tag, node.Line, node.Column)
		}

		if !isNullYAMLScalar(node) && node.Value != "" && !isWerfCiphertext(node.Value) {
			return fmt.Errorf("value at %q is not encrypted: encrypt the file with \"nelm chart secret values-file encrypt\"", strings.Join(keyPath, "."))
		}
	case yamlv3.AliasNode:
		// The anchored node is checked where it is defined.
	default:
		return fmt.Errorf("unexpected YAML node at line %d, column %d", node.Line, node.Column)
	}

	return nil
}

// migrateLegacyExtraAnnotationsAndLabels moves the extra_annotations.json and extra_labels.json
// of a chart published by werf <= 3.12 into the .werf-data metadata, classifying the annotations
// like werf bundle apply did.
func migrateLegacyExtraAnnotationsAndLabels(files []*chartcommon.File, data *werfdata.Data) ([]*chartcommon.File, error) {
	var result []*chartcommon.File
	for _, f := range files {
		switch f.Name {
		case legacyExtraAnnotationsFileName:
			annotations := map[string]string{}
			if err := json.Unmarshal(f.Data, &annotations); err != nil {
				return nil, fmt.Errorf("parse %s: %w", f.Name, err)
			}

			extraAnnotations := map[string]string{}

			serviceAnnotations := map[string]string{}
			for key, value := range annotations {
				if strings.HasPrefix(key, "project.werf.io/") || strings.Contains(key, "ci.werf.io/") || key == "werf.io/release-channel" {
					serviceAnnotations[key] = value
				} else {
					extraAnnotations[key] = value
				}
			}

			data.Metadata.ExtraAnnotations = mergeStringMaps(extraAnnotations, data.Metadata.ExtraAnnotations)
			data.Metadata.ExtraRuntimeAnnotations = mergeStringMaps(serviceAnnotations, data.Metadata.ExtraRuntimeAnnotations)
			data.Metadata.ReleaseInfoAnnotations = mergeStringMaps(serviceAnnotations, data.Metadata.ReleaseInfoAnnotations)
		case legacyExtraLabelsFileName:
			labels := map[string]string{}
			if err := json.Unmarshal(f.Data, &labels); err != nil {
				return nil, fmt.Errorf("parse %s: %w", f.Name, err)
			}

			data.Metadata.ExtraLabels = mergeStringMaps(labels, data.Metadata.ExtraLabels)
		default:
			result = append(result, f)
		}
	}

	return result, nil
}

// rewriteChartPackArchiveMetadata replaces Chart.yaml and Chart.lock of the root chart, which the
// archive writer generates from the parsed metadata, with the original files, so that their
// comments, key order and formatting survive. Only the edited Chart.yaml fields change.
func rewriteChartPackArchiveMetadata(ctx context.Context, archivePath string, chartAccessor helmchart.Accessor, edit chartyaml.EditOptions) error {
	replacements := map[string][]byte{}
	for _, f := range chartAccessor.RawFiles() {
		switch f.Name {
		case "Chart.yaml":
			chartYAML, err := chartyaml.Edit(ctx, f.Data, edit)
			if err != nil {
				return fmt.Errorf("edit Chart.yaml: %w", err)
			}

			replacements[path.Join(chartAccessor.Name(), f.Name)] = chartYAML
		case "Chart.lock":
			replacements[path.Join(chartAccessor.Name(), f.Name)] = f.Data
		}
	}

	if len(replacements) == 0 {
		return nil
	}

	return replaceArchiveFiles(archivePath, replacements)
}

// saveChartPackArchive applies the metadata edits to the loaded chart, so that a generated
// Chart.yaml, the archive name and the provenance get them, and saves the chart archive.
func saveChartPackArchive(loadedChart helmchart.Charter, files, werfDataFiles []*chartcommon.File, edit chartyaml.EditOptions, destination string) (string, error) {
	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		c.Metadata.Name = lo.CoalesceOrEmpty(edit.Name, c.Metadata.Name)
		c.Metadata.Version = lo.CoalesceOrEmpty(edit.Version, c.Metadata.Version)
		c.Metadata.AppVersion = lo.CoalesceOrEmpty(edit.AppVersion, c.Metadata.AppVersion)
		c.Metadata.Annotations = editAnnotationsMap(c.Metadata.Annotations, edit)
		c.Files = files
		c.WerfDataFiles = werfDataFiles
		c.Templates = withoutWerfHelpersTemplate(c.Templates)

		if _, err := semver.NewVersion(c.Metadata.Version); err != nil {
			return "", fmt.Errorf("parse chart version %q: %w", c.Metadata.Version, err)
		}

		archivePath, err := v2util.Save(c, destination)
		if err != nil {
			return "", fmt.Errorf("save chart: %w", err)
		}

		return archivePath, nil
	case *v3chart.Chart:
		c.Metadata.Name = lo.CoalesceOrEmpty(edit.Name, c.Metadata.Name)
		c.Metadata.Version = lo.CoalesceOrEmpty(edit.Version, c.Metadata.Version)
		c.Metadata.AppVersion = lo.CoalesceOrEmpty(edit.AppVersion, c.Metadata.AppVersion)
		c.Metadata.Annotations = editAnnotationsMap(c.Metadata.Annotations, edit)
		c.Files = files
		c.WerfDataFiles = werfDataFiles
		c.Templates = withoutWerfHelpersTemplate(c.Templates)

		if _, err := semver.NewVersion(c.Metadata.Version); err != nil {
			return "", fmt.Errorf("parse chart version %q: %w", c.Metadata.Version, err)
		}

		archivePath, err := v3util.Save(c, destination)
		if err != nil {
			return "", fmt.Errorf("save chart: %w", err)
		}

		return archivePath, nil
	default:
		panic(fmt.Sprintf("unexpected chart type %T", loadedChart))
	}
}

func applyChartPackOptionsDefaults(opts ChartPackOptions) (ChartPackOptions, error) {
	opts.ChartRepoConnectionOptions.ApplyDefaults()

	destination, err := filepath.Abs(opts.Destination)
	if err != nil {
		return ChartPackOptions{}, fmt.Errorf("get absolute path of destination %q: %w", opts.Destination, err)
	}

	opts.Destination = destination

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = filepath.Join(common.DefaultDockerConfig, "config.json")
	}

	if opts.RepositoryConfig == "" {
		opts.RepositoryConfig = helmpath.ConfigPath("repositories.yaml")
	}

	if opts.RepositoryCache == "" {
		opts.RepositoryCache = helmpath.CachePath("repository")
	}

	if opts.ContentCache == "" {
		opts.ContentCache = helmpath.CachePath("content")
	}

	return opts, nil
}

func chartAnnotations(loadedChart helmchart.Charter) map[string]string {
	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		return c.Metadata.Annotations
	case *v3chart.Chart:
		return c.Metadata.Annotations
	default:
		panic(fmt.Sprintf("unexpected chart type %T", loadedChart))
	}
}

func chartVersion(loadedChart helmchart.Charter) string {
	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		return c.Metadata.Version
	case *v3chart.Chart:
		return c.Metadata.Version
	default:
		panic(fmt.Sprintf("unexpected chart type %T", loadedChart))
	}
}

// checkChartPackArchiveSize fails for an archive that the chart loader would refuse for its
// decompressed size.
func checkChartPackArchiveSize(archivePath string) error {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archiveFile.Close()

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	defer gzipReader.Close()

	var totalSize int64

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		if header.Size > archive.MaxDecompressedFileSize {
			return fmt.Errorf("%s is %d bytes, more than the %d bytes a chart loader accepts for a single file", header.Name, header.Size, archive.MaxDecompressedFileSize)
		}

		totalSize += header.Size
	}

	if totalSize >= archive.MaxDecompressedChartSize {
		return fmt.Errorf("chart decompresses to %d bytes, a chart loader accepts less than %d bytes", totalSize, archive.MaxDecompressedChartSize)
	}

	return nil
}

func editAnnotationsMap(annotations map[string]string, edit chartyaml.EditOptions) map[string]string {
	result := lo.OmitByKeys(lo.Assign(annotations, edit.SetAnnotations), edit.DeleteAnnotations)
	if len(result) == 0 {
		return nil
	}

	return result
}

// isNullYAMLScalar reports whether node is a null scalar, explicitly tagged or not.
func isNullYAMLScalar(node *yamlv3.Node) bool {
	if node.Kind != yamlv3.ScalarNode || node.Tag != "!!null" {
		return false
	}

	return lo.Contains([]string{"", "~", "null", "Null", "NULL"}, node.Value)
}

// isWerfCiphertext checks the layout of a value encrypted by werf: hex of a two-byte
// little-endian format version followed by the IV or nonce and the ciphertext. It can't tell
// whether the value decrypts, which needs the key.
func isWerfCiphertext(value string) bool {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) < 2 {
		return false
	}

	payloadSize := len(decoded) - 2

	switch binary.LittleEndian.Uint16(decoded[:2]) {
	case 16:
		return payloadSize >= 32 && payloadSize%16 == 0
	case 2, 3:
		return payloadSize > 12+16
	default:
		return false
	}
}

func mergeStringMaps(base, overrides map[string]string) map[string]string {
	result := lo.Assign(base, overrides)
	if len(result) == 0 {
		return nil
	}

	return result
}

func newChartPackDependencyDownloader(ctx context.Context, chartPath string, registryClient *registry.Client, opts ChartPackOptions) *helmdownloader.Manager {
	return &helmdownloader.Manager{
		Out:               io.Discard,
		ChartPath:         chartPath,
		Debug:             log.Default.AcceptLevel(ctx, log.DebugLevel),
		SkipUpdate:        opts.ChartRepoSkipUpdate,
		Getters:           getter.Getters(),
		RegistryClient:    registryClient,
		RepositoryConfig:  opts.RepositoryConfig,
		RepositoryCache:   opts.RepositoryCache,
		ContentCache:      opts.ContentCache,
		AllowMissingRepos: true,
	}
}

func newChartPackRegistryClient(ctx context.Context, opts ChartPackOptions) (*registry.Client, error) {
	clientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
		registry.ClientOptBasicAuth(opts.ChartRepoBasicAuthUsername, opts.ChartRepoBasicAuthPassword),
	}

	if opts.ChartRepoCertPath != "" && opts.ChartRepoKeyPath != "" || opts.ChartRepoCAPath != "" || opts.ChartRepoSkipTLSVerify {
		tlsConfig, err := tlsutil.NewTLSConfig(
			tlsutil.WithInsecureSkipVerify(opts.ChartRepoSkipTLSVerify),
			tlsutil.WithCertKeyPairFiles(opts.ChartRepoCertPath, opts.ChartRepoKeyPath),
			tlsutil.WithCAFile(opts.ChartRepoCAPath),
		)
		if err != nil {
			return nil, fmt.Errorf("build TLS config: %w", err)
		}

		clientOpts = append(clientOpts, registry.ClientOptHTTPClient(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				Proxy:           http.ProxyFromEnvironment,
			},
		}))
	} else if opts.ChartRepoInsecure {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}

	client, err := registry.NewClient(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("create registry client: %w", err)
	}

	return client, nil
}

func nextWerfDataFileIndex(files map[string][]byte, dir string) int {
	next := 0
	for name := range files {
		index, err := strconv.Atoi(strings.TrimPrefix(name, dir+"/"))
		if !strings.HasPrefix(name, dir+"/") || err != nil {
			continue
		}

		next = max(next, index+1)
	}

	return next
}

// readChartPackInputFile reads a file passed to chart pack through the chart file reader of werf,
// if set, so that werf's giterminism rules apply to it.
func readChartPackInputFile(ctx context.Context, filePath string) ([]byte, error) {
	if common.ChartFileReader != nil {
		data, err := common.ChartFileReader.ReadChartFile(ctx, filePath)
		if err != nil {
			return nil, fmt.Errorf("read chart file: %w", err)
		}

		return data, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return data, nil
}

func replaceArchiveFiles(archivePath string, replacements map[string][]byte) error {
	original, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(original))
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	defer gzipReader.Close()

	tmpFile, err := os.CreateTemp(filepath.Dir(archivePath), filepath.Base(archivePath)+".tmp")
	if err != nil {
		return fmt.Errorf("create temporary archive: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	gzipWriter := gzip.NewWriter(tmpFile)
	gzipWriter.Header = gzipReader.Header

	tarReader := tar.NewReader(gzipReader)
	tarWriter := tar.NewWriter(gzipWriter)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		body, err := io.ReadAll(tarReader)
		if err != nil {
			return fmt.Errorf("read %q from archive: %w", header.Name, err)
		}

		if replacement, ok := replacements[header.Name]; ok {
			body = replacement
			header.Size = int64(len(body))
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write %q into archive: %w", header.Name, err)
		}

		if _, err := tarWriter.Write(body); err != nil {
			return fmt.Errorf("write %q into archive: %w", header.Name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), archivePath); err != nil {
		return fmt.Errorf("replace archive: %w", err)
	}

	return nil
}

// replaceFile moves src to dst. src is first moved, copying it across filesystems if needed, to a
// temporary file next to dst, which then replaces dst in one rename, so that dst is never left
// partly written.
func replaceFile(src, dst string) error {
	tmpPath := fmt.Sprintf("%s.tmp-%s", dst, uuid.NewString())

	if err := fs.RenameWithFallback(src, tmpPath); err != nil {
		os.Remove(tmpPath)

		return fmt.Errorf("move %q next to %q: %w", src, dst, err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)

		return fmt.Errorf("rename %q to %q: %w", tmpPath, dst, err)
	}

	return nil
}

// sanitizeYAMLError keeps only the line number of a YAML parse error, since the messages of YAML
// parsers may quote the source, which for secret values may be plaintext secrets.
func sanitizeYAMLError(err error) error {
	if match := yamlErrorLineRegexp.FindStringSubmatch(err.Error()); match != nil {
		return fmt.Errorf("invalid YAML at line %s", match[1])
	}

	return errors.New("invalid YAML")
}

// withoutWerfHelpersTemplate drops the empty placeholder template the loader adds to the root
// chart, which is filled only at render time.
func withoutWerfHelpersTemplate(templates []*chartcommon.File) []*chartcommon.File {
	return lo.Reject(templates, func(f *chartcommon.File, _ int) bool {
		return f.Name == "templates/_werf_helpers.tpl" && len(f.Data) == 0
	})
}

func yamlNodeKindName(kind yamlv3.Kind) string {
	switch kind {
	case yamlv3.SequenceNode:
		return "a list"
	case yamlv3.ScalarNode:
		return "a scalar"
	case yamlv3.AliasNode:
		return "an alias"
	default:
		return "an unexpected node"
	}
}
