package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/gofrs/flock"
	"github.com/samber/lo"
	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resource/schemas"
	"github.com/werf/nelm/pkg/resource/spec"
	"github.com/werf/nelm/pkg/util"
)

const (
	kubeConformCacheLockFilename       = "lock"
	kubeConformCacheMetadataAPIVersion = "v1"
	kubeConformCacheMetadataFilename   = "metadata.json"
)

type kubeConformValidator struct {
	cacheSubDirName     string
	embeddedSources     map[string]*schemas.Source
	schemaCacheLifetime time.Duration
	schemaSources       []string
	validators          []*kubeConformInstance
}

func newKubeConformValidator(schemaCacheLifetime time.Duration, schemaSources []string, embeddedSchemasOnly bool) (*kubeConformValidator, error) {
	kubernetesSource, err := schemas.KubernetesSource()
	if err != nil {
		return nil, fmt.Errorf("get embedded Kubernetes schemas: %w", err)
	}

	crdsSource, err := schemas.CRDsSource()
	if err != nil {
		return nil, fmt.Errorf("get embedded CRD schemas: %w", err)
	}

	if embeddedSchemasOnly {
		schemaSources = nil
	}

	cacheSubDirName := getHash(strings.Join(schemaSources, "-"))

	sources := slices.Clone(schemaSources)
	embeddedByTemplate := make(map[string]*schemas.Source, 2)

	for _, embeddedSource := range []*schemas.Source{kubernetesSource, crdsSource} {
		sources = append(sources, embeddedSource.Template)
		embeddedByTemplate[embeddedSource.Template] = embeddedSource
	}

	return &kubeConformValidator{
		cacheSubDirName:     cacheSubDirName,
		embeddedSources:     embeddedByTemplate,
		schemaCacheLifetime: schemaCacheLifetime,
		schemaSources:       sources,
	}, nil
}

func (kc *kubeConformValidator) Validate(ctx context.Context, resourceSpec *spec.ResourceSpec) error {
	yamlBytes, err := yaml.Marshal(resourceSpec.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to yaml: %w", err)
	}

	validators, err := kc.getValidatorInstances(ctx)
	if err != nil {
		return fmt.Errorf("get validators: %w", err)
	}

	matchedValidator, cacheEntryFound, err := kc.findCachedEntry(ctx, resourceSpec.GroupVersionKind)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	} else if cacheEntryFound {
		validators = []*kubeConformInstance{matchedValidator}
	}

validatorLoop:
	for _, schemaValidator := range validators {
		validationErrs := &util.MultiError{}

		resCh, errCh := resource.FromStream(ctx, "", bytes.NewReader(yamlBytes))

		for validationResource := range resCh {
			validationResult, err := schemaValidator.ValidateResource(ctx, validationResource)
			if err != nil {
				return fmt.Errorf("schema validator: %w", err)
			}

			switch validationResult.Status {
			case validator.Error:
				if strings.HasPrefix(validationResult.Err.Error(), "could not find schema") {
					continue validatorLoop
				}

				return validationResult.Err
			case validator.Skipped:
				log.Default.Debug(ctx, "Skip validation for resource: %s", resourceSpec.IDHuman())
			case validator.Invalid:
				if !cacheEntryFound {
					if err := schemaValidator.AddCacheEntry(ctx, resourceSpec.GroupVersionKind); err != nil {
						return fmt.Errorf("add entry %s: %w", resourceSpec.IDHuman(), err)
					}
				}

				for _, validationErr := range validationResult.ValidationErrors {
					validationErrs.Add(fmt.Errorf("%s: %w", validationErr.Path, &validationErr))
				}
			case validator.Valid:
				if !cacheEntryFound {
					if err := schemaValidator.AddCacheEntry(ctx, resourceSpec.GroupVersionKind); err != nil {
						return fmt.Errorf("add entry %s: %w", resourceSpec.IDHuman(), err)
					}
				}
			default:
				panic(fmt.Errorf("unexpected validation status %q", validationResult.Status))
			}
		}

		// Check for stream reading errors
		for err := range errCh {
			if err != nil {
				return fmt.Errorf("read resource stream: %w", err)
			}
		}

		return validationErrs.OrNilIfNoErrs()
	}

	return nil
}

func (kc *kubeConformValidator) findCachedEntry(ctx context.Context, gvk schema.GroupVersionKind) (*kubeConformInstance, bool, error) {
	for _, v := range kc.validators {
		found, err := v.FindCachedEntry(ctx, gvk)
		if err != nil {
			return nil, false, fmt.Errorf("find entry %s: %w", gvk, err)
		}

		if found {
			return v, true, nil
		}
	}

	return nil, false, nil
}

func (kc *kubeConformValidator) getValidatorInstances(ctx context.Context) ([]*kubeConformInstance, error) {
	if len(kc.validators) > 0 {
		return kc.validators, nil
	}

	// Not configurable: it is what the embedded schemas were generated for. kubeconform resolves
	// "{{ .NormalizedKubernetesVersion }}" with it, and it keys the cache entries.
	kubeVersion, err := schemas.KubeVersion()
	if err != nil {
		return nil, fmt.Errorf("get Kubernetes version of the embedded schemas: %w", err)
	}

	for _, source := range kc.schemaSources {
		cacheDir, err := createKubeConformCacheDir(kc.cacheSubDirName, source)
		if err != nil {
			return nil, fmt.Errorf("get schema cache dir: %w", err)
		}

		validationInstance, err := newKubeConformInstance(ctx, source, cacheDir, kubeVersion, kc.schemaCacheLifetime, kc.embeddedSources[source])
		if err != nil {
			return nil, fmt.Errorf("get generic validator: %w", err)
		}

		if err := validationInstance.InvalidateCacheEntries(ctx); err != nil {
			return nil, fmt.Errorf("invalidate validator cache: %w", err)
		}

		kc.validators = append(kc.validators, validationInstance)
	}

	return kc.validators, nil
}

type kubeConformCacheEntry struct {
	Created    time.Time `json:"created"`
	SchemaFile string    `json:"schemaFile,omitempty"`
}

type kubeConformCacheMetadata struct {
	APIVersion string                           `json:"apiVersion"`
	Entries    map[string]kubeConformCacheEntry `json:"entries"`
}

func newKubeConformCacheMetadata() *kubeConformCacheMetadata {
	return &kubeConformCacheMetadata{
		APIVersion: kubeConformCacheMetadataAPIVersion,
		Entries:    make(map[string]kubeConformCacheEntry),
	}
}

type kubeConformInstance struct {
	cacheDir       string
	cacheLifetime  time.Duration
	embeddedSource *schemas.Source
	fileLock       *flock.Flock
	kubeVersion    string
	metadata       kubeConformCacheMetadata
	source         string
	validator      validator.Validator
}

func newKubeConformInstance(ctx context.Context, source, cacheDir, kubeVersion string, cacheLifetime time.Duration, embeddedSource *schemas.Source) (*kubeConformInstance, error) {
	validatorOpts := validator.Opts{
		Strict:               false,
		IgnoreMissingSchemas: false,
		KubernetesVersion:    kubeVersion,
		Cache:                cacheDir,
	}

	if isLocalFSSource(source) {
		// Disable kubeconform integrated caching for local file system sources.
		validatorOpts.Cache = ""
	}

	if log.Default.AcceptLevel(ctx, log.DebugLevel) {
		validatorOpts.Debug = true
	}

	validatorInstance, err := validator.New([]string{source}, validatorOpts)
	if err != nil {
		return nil, fmt.Errorf("create schema validator: %w", err)
	}

	lockFilePath := filepath.Join(cacheDir, kubeConformCacheLockFilename)

	v := &kubeConformInstance{
		cacheDir:       cacheDir,
		cacheLifetime:  cacheLifetime,
		embeddedSource: embeddedSource,
		fileLock:       flock.New(lockFilePath),
		kubeVersion:    kubeVersion,
		source:         source,
		validator:      validatorInstance,
	}

	if err := v.fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("acquire lock on schema validator %s: %w", lockFilePath, err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	metadata, err := readKubeConformMetadata(ctx, v.metadataFilePath())
	if err != nil {
		return nil, fmt.Errorf("read kube conform metadata: %w", err)
	}

	v.metadata = *metadata

	return v, nil
}

func (v *kubeConformInstance) AddCacheEntry(ctx context.Context, gvk schema.GroupVersionKind) error {
	if err := v.fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	metadata, err := readKubeConformMetadata(ctx, v.metadataFilePath())
	if err != nil {
		return fmt.Errorf("load metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata = *metadata

	schemaFile, err := v.schemaCacheFileName(gvk)
	if err != nil {
		return fmt.Errorf("get schema cache file name for %s: %w", gvk, err)
	}

	v.metadata.Entries[getKubeConformEntryHash(v.kubeVersion, gvk)] = kubeConformCacheEntry{
		Created:    time.Now().UTC(),
		SchemaFile: schemaFile,
	}

	if err := writeKubeConformCacheMetadata(v.metadataFilePath(), v.metadata); err != nil {
		return fmt.Errorf("write metadata %s: %w", v.metadataFilePath(), err)
	}

	return nil
}

func (v *kubeConformInstance) FindCachedEntry(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	if err := v.fileLock.Lock(); err != nil {
		return false, fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	metadata, err := readKubeConformMetadata(ctx, v.metadataFilePath())
	if err != nil {
		return false, fmt.Errorf("load metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata = *metadata

	// Do not invalidate cache to avoid connectivity issues that could lead
	// to validation inability of remaining resources.
	_, found := v.metadata.Entries[getKubeConformEntryHash(v.kubeVersion, gvk)]
	if !found {
		return false, nil
	}

	return found, nil
}

func (v *kubeConformInstance) InvalidateCacheEntries(ctx context.Context) error {
	if err := v.fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	metadata, err := readKubeConformMetadata(ctx, v.metadataFilePath())
	if err != nil {
		return fmt.Errorf("refresh metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata = *metadata

	var changed bool

	for hash, entry := range v.metadata.Entries {
		if entry.Created.Add(v.cacheLifetime).After(time.Now().UTC()) {
			continue
		}

		// Dropping the file is what makes the lifetime mean anything: kubeconform never overwrites a
		// schema it has cached, so while the file is there it is reused no matter how old.
		if entry.SchemaFile != "" {
			schemaFilePath := filepath.Join(v.cacheDir, entry.SchemaFile)

			if err := os.Remove(schemaFilePath); err != nil {
				if os.IsNotExist(err) {
					// Normal: another process may have evicted it, or this source never had the schema.
					log.Default.Debug(ctx, "Cached schema %s is already gone, nothing to evict", schemaFilePath)
				} else {
					log.Default.Warn(ctx, "Cannot remove cached schema %s: %s", schemaFilePath, err)
				}
			}
		}

		log.Default.Debug(ctx, "Invalidating schema validator cache entry %s", hash)
		delete(v.metadata.Entries, hash)

		changed = true
	}

	if changed {
		if err := writeKubeConformCacheMetadata(v.metadataFilePath(), v.metadata); err != nil {
			return fmt.Errorf("write metadata: %w", err)
		}
	}

	return nil
}

func (v *kubeConformInstance) ValidateResource(ctx context.Context, res resource.Resource) (*validator.Result, error) {
	if err := v.ensureReady(ctx); err != nil {
		return nil, err
	}

	if err := v.fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	return lo.ToPtr(v.validator.ValidateResource(res)), nil
}

// ensureReady unpacks the embedded bundle right before it is first read. A no-op for other sources.
func (v *kubeConformInstance) ensureReady(ctx context.Context) error {
	if v.embeddedSource == nil {
		return nil
	}

	if err := v.embeddedSource.EnsureExtracted(ctx); err != nil {
		return fmt.Errorf("unpack embedded schemas: %w", err)
	}

	return nil
}

func (v *kubeConformInstance) lockFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheLockFilename)
}

func (v *kubeConformInstance) metadataFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheMetadataFilename)
}

// schemaCacheFileName returns the file kubeconform caches this resource's schema in, which is the hash
// of the schema location. Local sources are read in place and never cached, so they get no name.
func (v *kubeConformInstance) schemaCacheFileName(gvk schema.GroupVersionKind) (string, error) {
	if isLocalFSSource(v.source) {
		return "", nil
	}

	schemaLocation, err := patchKubeConformSchemaSource(v.source, gvk, false, v.kubeVersion)
	if err != nil {
		return "", fmt.Errorf("patch schema source %s: %w", v.source, err)
	}

	return getHash(schemaLocation), nil
}

func createKubeConformCacheDir(subDir, source string) (string, error) {
	sourceHash := getHash(source)

	var sourceDirName string

	if isLocalFSSource(source) {
		sourceDirName = "local-" + sourceHash[:7]
	} else {
		u, err := url.Parse(source)
		if err != nil {
			return "", fmt.Errorf("parse source url %q: %w", source, err)
		}

		sourceDirName = u.Hostname() + "-" + sourceHash[:7]
	}

	path := filepath.Join(helmpath.CachePath(common.CacheDirAPIResourceJSONSchemas), subDir, sourceDirName)

	if stat, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", fmt.Errorf("create cache dir %q: %w", path, err)
		}

		return path, nil
	} else if err != nil {
		return "", fmt.Errorf("stat cache dir %q: %w", path, err)
	} else if !stat.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}

	return path, nil
}

func getKubeConformEntryHash(kubeVersion string, gvk schema.GroupVersionKind) string {
	return getHash(fmt.Sprintf("%s-%s-%s", gvk.Kind, gvk.GroupVersion(), kubeVersion))
}

func getHash(s string) string {
	digest := sha256.Sum256([]byte(s))

	return hex.EncodeToString(digest[:])
}

func isLocalFSSource(source string) bool {
	return !strings.HasPrefix(source, "https://") && !strings.HasPrefix(source, "http://")
}

// patchKubeConformSchemaSource renders a source template into the schema location for a resource. It
// mirrors the unexported schemaPath of kubeconform's registry, byte for byte.
func patchKubeConformSchemaSource(source string, gvk schema.GroupVersionKind, strict bool, kubeVersion string) (string, error) {
	// Recombined the way kubeconform sees it, as a raw apiVersion: "apps/v1", or "v1" for core.
	groupParts := strings.Split(gvk.GroupVersion().String(), "/")
	versionParts := strings.Split(groupParts[0], ".")

	kindSuffix := "-" + strings.ToLower(versionParts[0])
	if len(groupParts) > 1 {
		kindSuffix += "-" + strings.ToLower(groupParts[1])
	}

	params := struct {
		Group                       string
		NormalizedKubernetesVersion string
		ResourceAPIVersion          string
		ResourceKind                string
		StrictSuffix                string
		KindSuffix                  string
	}{
		Group:                       groupParts[0],
		NormalizedKubernetesVersion: kubeVersion,
		ResourceAPIVersion:          groupParts[len(groupParts)-1],
		ResourceKind:                strings.ToLower(gvk.Kind),
		KindSuffix:                  kindSuffix,
	}

	if kubeVersion != "master" {
		params.NormalizedKubernetesVersion = "v" + kubeVersion
	}

	if strict {
		params.StrictSuffix = "-strict"
	}

	// kubeconform appends its own layout to any source not already pointing at a file, remote and local
	// alike. Skipping that here would make the cache entry name a file that does not exist, and the
	// expiry then drops nothing. Mirrored verbatim, trailing slash and all.
	if !strings.HasSuffix(source, "json") {
		source += "/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"
	}

	tmpl, err := template.New("tpl").Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse schema source: %w", err)
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("execute schema source template: %w", err)
	}

	return buf.String(), nil
}

// readKubeConformMetadata falls back to an empty metadata whenever the file cannot be made sense of:
// it is a pure optimization, so a broken one must cost a few extra lookups, not fail the run.
func readKubeConformMetadata(ctx context.Context, path string) (*kubeConformCacheMetadata, error) {
	metadataBytes, err := os.ReadFile(path)
	if err != nil {
		// Anything but a missing file points at the environment, not at the cache contents.
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		return newKubeConformCacheMetadata(), nil
	}

	var metadata kubeConformCacheMetadata

	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		log.Default.Warn(ctx, "Resetting unreadable schema cache metadata %s: %s", path, err)

		return newKubeConformCacheMetadata(), nil
	}

	if metadata.APIVersion != kubeConformCacheMetadataAPIVersion {
		log.Default.Warn(ctx, "Resetting schema cache metadata %s written in unsupported format %q", path, metadata.APIVersion)

		return newKubeConformCacheMetadata(), nil
	}

	// A metadata file with no entries at all decodes into a nil map, which is not writable.
	if metadata.Entries == nil {
		metadata.Entries = make(map[string]kubeConformCacheEntry)
	}

	return &metadata, nil
}

// writeKubeConformCacheMetadata writes in full and moves into place, so an interrupted write leaves
// the previous metadata intact instead of a truncated file for the next run to choke on.
func writeKubeConformCacheMetadata(path string, metadata kubeConformCacheMetadata) error {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode metadata for %s: %w", path, err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}

	tmpPath := tmpFile.Name()

	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(metadataBytes); err != nil {
		tmpFile.Close()

		return fmt.Errorf("write %s: %w", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("move %s to %s: %w", tmpPath, path, err)
	}

	return nil
}
