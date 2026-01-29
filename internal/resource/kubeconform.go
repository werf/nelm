package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/gofrs/flock"
	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	kubeConformCacheLockFilename       = "lock"
	kubeConformCacheMetadataAPIVersion = "1"
	kubeConformCacheMetadataFilename   = "metadata.json"
)

type KubeConformValidator struct {
	kubeVersion         string
	schemaCacheLifetime time.Duration
	schemaSources       []string
	validators          []*kubeConformValidatorInstance
}

func NewKubeConformValidator(kubeVersion string, schemaCacheLifetime time.Duration, schemaSource []string) (*KubeConformValidator, error) {
	return &KubeConformValidator{
		kubeVersion:         strings.TrimLeft(kubeVersion, "v"),
		schemaCacheLifetime: schemaCacheLifetime,
		schemaSources:       schemaSource,
	}, nil
}

func (kc *KubeConformValidator) Validate(ctx context.Context, res *InstallableResource) error {
	yamlBytes, err := yaml.Marshal(res.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to yaml: %w", err)
	}

	validators, err := kc.getValidatorInstances(ctx)
	if err != nil {
		return fmt.Errorf("get validators: %w", err)
	}

	var cachedEntryFound bool
	if matchedValidator, found, err := kc.findCachedEntry(ctx, res.GroupVersionKind); err != nil {
		return fmt.Errorf("get validator: %w", err)
	} else if found {
		cachedEntryFound = true
		validators = []*kubeConformValidatorInstance{matchedValidator}
	}

VALIDATOR:
	for _, schemaValidator := range validators {
		validationErrs := &util.MultiError{}

		resCh, errCh := resource.FromStream(ctx, res.FilePath, bytes.NewReader(yamlBytes))

		for validationResource := range resCh {
			validationResult, err := schemaValidator.ValidateResource(ctx, validationResource)
			if err != nil {
				return fmt.Errorf("schema validator: %w", err)
			}

			switch validationResult.Status {
			case validator.Error:
				if strings.HasPrefix(validationResult.Err.Error(), "could not find schema") {
					continue VALIDATOR
				}

				return validationResult.Err
			case validator.Skipped:
				log.Default.Debug(ctx, "Skip validation for resource: %s", res.IDHuman())
			case validator.Invalid:
				if !cachedEntryFound {
					if err := schemaValidator.AddCacheEntry(ctx, res.GroupVersionKind); err != nil {
						return fmt.Errorf("add entry %s: %w", res.IDHuman(), err)
					}
				}

				for _, validationErr := range validationResult.ValidationErrors {
					validationErrs.Add(fmt.Errorf("%s: %w", validationErr.Path, &validationErr))
				}
			case validator.Empty, validator.Valid:
				if !cachedEntryFound {
					if err := schemaValidator.AddCacheEntry(ctx, res.GroupVersionKind); err != nil {
						return fmt.Errorf("add entry %s: %w", res.IDHuman(), err)
					}
				}

				continue
			default:
				panic("unexpected validation status")
			}
		}

		// Check for stream reading Errors
		for err := range errCh {
			if err != nil {
				return fmt.Errorf("read resource stream: %w", err)
			}
		}

		return validationErrs.OrNilIfNoErrs()
	}

	return nil
}

func (kc *KubeConformValidator) findCachedEntry(ctx context.Context, gvk schema.GroupVersionKind) (*kubeConformValidatorInstance, bool, error) {
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

func (kc *KubeConformValidator) getValidatorInstances(ctx context.Context) ([]*kubeConformValidatorInstance, error) {
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

		validationInstance, err := newKubeConformValidatorInstance(ctx, source, cacheDir, kc.kubeVersion, kc.schemaCacheLifetime)
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
func (kc *KubeConformValidator) validateSchemasSources(ctx context.Context) error {
	schemaSourceParams := kubeConformSchemaSourceParams{
		Group:                       "apps",
		NormalizedKubernetesVersion: "v" + kc.kubeVersion,
		ResourceAPIVersion:          "v1",
		ResourceKind:                "deployment",
		StrictSuffix:                "",
	}

	errs := &util.MultiError{}

	for _, schemaSource := range kc.schemaSources {
		patchedSource, err := patchKubeConformSchemaSource(schemaSource, schemaSourceParams)
		if err != nil {
			errs.Add(fmt.Errorf("patch schema source with test params: %w", err))

			continue
		}

		if isLocalFSSource(schemaSource) {
			if _, err := os.Stat(schemaSource); err != nil {
				errs.Add(fmt.Errorf("open test schema %s: %w", patchedSource, err))

				continue
			}

			return nil
		}

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, patchedSource, nil)
		if err != nil {
			errs.Add(fmt.Errorf("connect to download test schema: %w", err))

			continue
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			errs.Add(fmt.Errorf("download test schema: %w", err))

			continue
		}

		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			errs.Add(fmt.Errorf("test schema %s for kube version %s not found", patchedSource, kc.kubeVersion))

			continue
		}

		return nil
	}

	return errs
}

type kubeConformSchemaSourceParams struct {
	Group                       string
	KindSuffix                  string
	NormalizedKubernetesVersion string
	ResourceAPIVersion          string
	ResourceKind                string
	StrictSuffix                string
}

type kubeConformCacheMetadata struct {
	APIVersion string               `json:"apiVersion"`
	Entries    map[string]time.Time `json:"entries"`
}

type kubeConformValidatorInstance struct {
	cacheDir      string
	cacheLifetime time.Duration
	fileLock      *flock.Flock
	metadata      kubeConformCacheMetadata
	source        string
	kubeVersion   string
	validator     validator.Validator
}

func newKubeConformValidatorInstance(ctx context.Context, source, cacheDir, kubeVersion string, cacheLifetime time.Duration) (*kubeConformValidatorInstance, error) {
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

	v := &kubeConformValidatorInstance{
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
		metadataFile, err := os.OpenFile(metadataFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, fmt.Errorf("unable to open metadata file: %w", err)
		}

		defer metadataFile.Close()

		meta := kubeConformCacheMetadata{
			APIVersion: kubeConformCacheMetadataAPIVersion,
			Entries:    make(map[string]time.Time),
		}

		encoder := json.NewEncoder(metadataFile)

		if err := encoder.Encode(meta); err != nil {
			return nil, fmt.Errorf("encode %s: %w", metadataFilePath, err)
		}

		v.metadata = meta

		return v, nil
	}

	metadataFile, err := os.OpenFile(metadataFilePath, os.O_RDONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", metadataFilePath, err)
	}

	defer metadataFile.Close()

	decoder := json.NewDecoder(metadataFile)

	if err := decoder.Decode(&v.metadata); err != nil {
		return nil, fmt.Errorf("decode metadata from %s: %w", metadataFilePath, err)
	}

	if v.metadata.APIVersion != kubeConformCacheMetadataAPIVersion {
		return nil, fmt.Errorf("invalid metadata API version %q found in %s", v.metadata.APIVersion, metadataFilePath)
	}

	return v, nil
}

func (v *kubeConformValidatorInstance) AddCacheEntry(ctx context.Context, gvk schema.GroupVersionKind) error {
	if err := v.fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	if err := v.loadMetadataFromDisk(); err != nil {
		return fmt.Errorf("load metadata from %s: %w", v.metadataFilePath(), err)
	}

	v.metadata.Entries[getKubeConformEntryHash(v.kubeVersion, gvk)] = time.Now().UTC()

	if err := v.dumpMetadata(); err != nil {
		return fmt.Errorf("dump metadata %s: %w", v.metadataFilePath(), err)
	}

	return nil
}

func (v *kubeConformValidatorInstance) FindCachedEntry(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	if err := v.fileLock.Lock(); err != nil {
		return false, fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	if err := v.loadMetadataFromDisk(); err != nil {
		return false, fmt.Errorf("load metadata from %s: %w", v.metadataFilePath(), err)
	}

	// Do not invalidate cache to avoid connectivity issues that could lead
	// to validation inability of remaining resources.
	_, found := v.metadata.Entries[getKubeConformEntryHash(v.kubeVersion, gvk)]
	if !found {
		return false, nil
	}

	return found, nil
}

func (v *kubeConformValidatorInstance) InvalidateCacheEntries(ctx context.Context) error {
	if err := v.fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	if err := v.loadMetadataFromDisk(); err != nil {
		return fmt.Errorf("refresh metadata from %s: %w", v.metadataFilePath(), err)
	}

	var changed bool

	for hash, timestamp := range v.metadata.Entries {
		if timestamp.Add(v.cacheLifetime).Before(time.Now().UTC()) {
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
	}

	if changed {
		if err := v.dumpMetadata(); err != nil {
			return fmt.Errorf("dump metadata: %w", err)
		}
	}

	return nil
}

func (v *kubeConformValidatorInstance) ValidateResource(ctx context.Context, res resource.Resource) (*validator.Result, error) {
	if err := v.fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("acquire lock on schema validator %s: %w", v.lockFilePath(), err)
	}

	defer func() {
		if err := v.fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on schema validator %s: %s", v.lockFilePath(), err)
		}
	}()

	// Underlying calls has thread unsafe os.WriteFile call, so we must protect it by file lock.
	result := v.validator.ValidateResource(res)

	return &result, nil
}

func (v *kubeConformValidatorInstance) dumpMetadata() error {
	metadataFile, err := os.OpenFile(v.metadataFilePath(), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", v.metadataFilePath(), err)
	}

	defer metadataFile.Close()

	encoder := json.NewEncoder(metadataFile)
	if err := encoder.Encode(v.metadata); err != nil {
		return fmt.Errorf("update %s: %w", v.metadataFilePath(), err)
	}

	return nil
}

func (v *kubeConformValidatorInstance) loadMetadataFromDisk() error {
	var persistedMetadata kubeConformCacheMetadata

	metadataFile, err := os.OpenFile(v.metadataFilePath(), os.O_RDONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", v.metadataFilePath(), err)
	}

	defer metadataFile.Close()

	decoder := json.NewDecoder(metadataFile)

	if err := decoder.Decode(&persistedMetadata); err != nil {
		return fmt.Errorf("decode persisted metadata %s: %w", v.metadataFilePath(), err)
	}

	if persistedMetadata.APIVersion != kubeConformCacheMetadataAPIVersion {
		return fmt.Errorf("invalid metadata API version %q found in %s", persistedMetadata.APIVersion, v.metadataFilePath())
	}

	for key, value := range persistedMetadata.Entries {
		v.metadata.Entries[key] = value
	}

	return nil
}

func (v *kubeConformValidatorInstance) lockFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheLockFilename)
}

func (v *kubeConformValidatorInstance) metadataFilePath() string {
	return filepath.Join(v.cacheDir, kubeConformCacheMetadataFilename)
}

func createKubeConformCacheDir(subDir, source string) (string, error) {
	sourceHash := getHash(source)

	sourceDirName := "local-" + sourceHash[:7]

	u, err := url.Parse(source)
	if err == nil && u.Hostname() != "" {
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

func getHash(s string) string {
	digest := sha256.Sum256([]byte(s))

	return hex.EncodeToString(digest[:])
}

func patchKubeConformSchemaSource(source string, params kubeConformSchemaSourceParams) (string, error) {
	var buf bytes.Buffer

	tmpl, err := template.New("tpl").Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse schema source: %w", err)
	}

	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("execute schema source template: %w", err)
	}

	return buf.String(), nil
}

func isLocalFSSource(source string) bool {
	return !strings.HasPrefix(source, "https://") && !strings.HasPrefix(source, "http://")
}
