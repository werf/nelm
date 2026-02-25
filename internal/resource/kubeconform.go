package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/gofrs/flock"
	"github.com/samber/lo"
	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	kubeConformCacheLockFilename       = "lock"
	kubeConformCacheMetadataAPIVersion = "v1"
	kubeConformCacheMetadataFilename   = "metadata.json"
)

var ErrResourceValidationSourceSanityCheck = errors.New("resource validation source sanity check")

type kubeConformValidator struct {
	kubeVersion         string
	schemaCacheLifetime time.Duration
	schemaSources       []string
	validators          []*kubeConformInstance
}

func newKubeConformValidator(kubeVersion string, schemaCacheLifetime time.Duration, schemaSource []string) (*kubeConformValidator, error) {
	return &kubeConformValidator{
		kubeVersion:         strings.TrimLeft(kubeVersion, "v"),
		schemaCacheLifetime: schemaCacheLifetime,
		schemaSources:       schemaSource,
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
	// TODO(major): if possible, we should use only a single yaml marshaller and a single json marshaller everywhere
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

	if err := kc.validateSchemasSources(ctx); err != nil {
		return nil, fmt.Errorf("validate schema sources: %w", err)
	}

	// Generate top level directory name based on source combination, to avoid invalid cache hit on
	// source combination change.
	sourcesSubDirName := getHash(strings.Join(kc.schemaSources, "-"))

	for _, source := range kc.schemaSources {
		cacheDir, err := createKubeConformCacheDir(sourcesSubDirName, source)
		if err != nil {
			return nil, fmt.Errorf("get schema cache dir: %w", err)
		}

		validationInstance, err := newKubeConformInstance(ctx, source, cacheDir, kc.kubeVersion, kc.schemaCacheLifetime)
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

// validateSchemasSources validates source against Kubernetes version based on assumption that
// every Kubernetes version has Deployment.apps/v1 schema. If at least one source statisfy to condition, the complete
// source list considered to be valid, as it allows to validate native resources.
func (kc *kubeConformValidator) validateSchemasSources(ctx context.Context) error {
	httpClient := util.NewRestyClient(ctx)

	for _, schemaSource := range kc.schemaSources {
		patchedSource, err := patchKubeConformSchemaSource(schemaSource, "deployment",
			"apps", "v1", false, kc.kubeVersion)
		if err != nil {
			return fmt.Errorf("%w: patch schema source %s: %w", ErrResourceValidationSourceSanityCheck, schemaSource, err)
		}

		if isLocalFSSource(patchedSource) {
			if _, err := os.Stat(patchedSource); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("%w: open test schema for Deployment/apps/v1 for kube version %s: %w", ErrResourceValidationSourceSanityCheck, kc.kubeVersion, err)
				}

				log.Default.Debug(ctx, "Test schema for Deployment/apps/v1 for kube version %s not found at %s: %w", kc.kubeVersion, patchedSource, err)

				continue
			}

			return nil
		}

		response, err := httpClient.R().SetContext(ctx).Head(patchedSource)
		if err != nil {
			return fmt.Errorf("%w: cannot get test schema for deployment/apps/v1 for kube version %s at %s: %w",
				ErrResourceValidationSourceSanityCheck, kc.kubeVersion, patchedSource, err)
		}

		switch response.StatusCode() {
		case http.StatusOK:
			return nil
		case http.StatusNotFound:
			log.Default.Debug(ctx, "Test schema for Deployment/apps/v1 for kube version %s not found at %s not found", kc.kubeVersion, patchedSource)

			continue
		default:
			return fmt.Errorf("%w: got unexpected status code %d", ErrResourceValidationSourceSanityCheck, response.StatusCode())
		}
	}

	return fmt.Errorf("%w: unable to get deployment/apps/v1 for kube version %s in any schema sources", ErrResourceValidationSourceSanityCheck, kc.kubeVersion)
}

type kubeConformCacheEntry struct {
	Created time.Time `json:"created"`
}

type kubeConformCacheMetadata struct {
	APIVersion string                           `json:"apiVersion"`
	Entries    map[string]kubeConformCacheEntry `json:"entries"`
}

type kubeConformInstance struct {
	cacheDir      string
	cacheLifetime time.Duration
	fileLock      *flock.Flock
	kubeVersion   string
	metadata      kubeConformCacheMetadata
	source        string
	validator     validator.Validator
}

func newKubeConformInstance(ctx context.Context, source, cacheDir, kubeVersion string, cacheLifetime time.Duration) (*kubeConformInstance, error) {
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
		cacheDir:      cacheDir,
		cacheLifetime: cacheLifetime,
		fileLock:      flock.New(lockFilePath),
		kubeVersion:   kubeVersion,
		source:        source,
		validator:     validatorInstance,
	}

	if err := v.fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("acquire lock on schema validator %s: %w", lockFilePath, err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	metadataFilePath := filepath.Join(cacheDir, kubeConformCacheMetadataFilename)

	if _, err := os.Stat(metadataFilePath); os.IsNotExist(err) {
		v.metadata = kubeConformCacheMetadata{
			APIVersion: kubeConformCacheMetadataAPIVersion,
			Entries:    make(map[string]kubeConformCacheEntry),
		}

		if err := writeKubeConformCacheMetadata(metadataFilePath, v.metadata); err != nil {
			return nil, fmt.Errorf("write kube conform cache metadata: %w", err)
		}

		return v, nil
	}

	metadata, err := readKubeConformMetadata(metadataFilePath)
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

	metadata, err := readKubeConformMetadata(v.metadataFilePath())
	if err != nil {
		return fmt.Errorf("load metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata = *metadata

	v.metadata.Entries[getKubeConformEntryHash(v.kubeVersion, gvk)] = kubeConformCacheEntry{
		Created: time.Now().UTC(),
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

	metadata, err := readKubeConformMetadata(v.metadataFilePath())
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

	metadata, err := readKubeConformMetadata(v.metadataFilePath())
	if err != nil {
		return fmt.Errorf("refresh metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata = *metadata

	var changed bool

	for hash, entry := range v.metadata.Entries {
		if entry.Created.Add(v.cacheLifetime).After(time.Now().UTC()) {
			continue
		}

		entryFilePath := filepath.Join(v.cacheDir, hash)

		if !isLocalFSSource(v.source) {
			if err := os.Remove(entryFilePath); err != nil {
				log.Default.Warn(ctx, "Cannot remove schema cache entry %s: %s", entryFilePath, err)
			}
		}

		log.Default.Debug(ctx, "Invalidating schema validator cache entry %s", entryFilePath)
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

func (v *kubeConformInstance) lockFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheLockFilename)
}

func (v *kubeConformInstance) metadataFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheMetadataFilename)
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

	path := filepath.Join(common.APIResourceValidationJSONSchemasCacheDir, subDir, sourceDirName)

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

func patchKubeConformSchemaSource(source, kind, group, apiVersion string, strict bool, kubeVersion string) (string, error) {
	kindSuffix := "-" + group + "-" + apiVersion
	if group == "" {
		kindSuffix = "-" + kubeVersion
	}

	params := struct {
		Group                       string
		NormalizedKubernetesVersion string
		ResourceAPIVersion          string
		ResourceKind                string
		StrictSuffix                string
		KindSuffix                  string
	}{
		Group:                       group,
		NormalizedKubernetesVersion: kubeVersion,
		ResourceAPIVersion:          apiVersion,
		ResourceKind:                kind,
		KindSuffix:                  kindSuffix,
	}

	if kubeVersion != "master" {
		params.NormalizedKubernetesVersion = "v" + kubeVersion
	}

	if strict {
		params.StrictSuffix = "-strict"
	}

	if isLocalFSSource(source) && !strings.HasSuffix(source, ".json") {
		// This local path adjustments match the default kubeconform logic.
		source = strings.TrimRight(source, "/") + "/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"
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

func getHash(s string) string {
	digest := sha256.Sum256([]byte(s))

	return hex.EncodeToString(digest[:])
}

func isLocalFSSource(source string) bool {
	return !strings.HasPrefix(source, "https://") && !strings.HasPrefix(source, "http://")
}

func readKubeConformMetadata(path string) (*kubeConformCacheMetadata, error) {
	var metadata kubeConformCacheMetadata

	metadataFile, err := os.OpenFile(path, os.O_RDONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	defer metadataFile.Close()

	decoder := json.NewDecoder(metadataFile)

	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode persisted metadata %s: %w", path, err)
	}

	if metadata.APIVersion != kubeConformCacheMetadataAPIVersion {
		return nil, fmt.Errorf("invalid metadata API version %q found in %s", metadata.APIVersion, path)
	}

	return &metadata, nil
}

func writeKubeConformCacheMetadata(path string, metadata kubeConformCacheMetadata) error {
	metadataFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	defer metadataFile.Close()

	encoder := json.NewEncoder(metadataFile)
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}

	return nil
}
