package secret

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/log"
)

var (
	_ RuntimeData = (*SecretsRuntimeData)(nil)

	simpleSecretValuePathKeyRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

type DecodeAndLoadSecretsOptions = chartcommon.DecodeAndLoadSecretsOptions

type RuntimeData interface {
	DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*common.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error
	GetEncodedSecretValues(ctx context.Context, secretsManager *secrets_manager.SecretsManager, secretsWorkingDir string, noDecryptSecrets bool) (map[string]interface{}, error)
	GetDecryptedDefaultSecretValues() map[string]interface{}
	GetDecryptedCustomSecretValues() map[string]interface{}
	GetDecryptedSecretFilesData() map[string]string
}

type SecretsRuntimeData struct {
	decryptedCustomSecretValues  map[string]interface{}
	decryptedDefaultSecretValues map[string]interface{}
	decryptedSecretFilesData     map[string]string
}

func NewSecretsRuntimeData() *SecretsRuntimeData {
	return &SecretsRuntimeData{
		decryptedSecretFilesData: make(map[string]string),
	}
}

func (s *SecretsRuntimeData) DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*common.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error {
	secretDirFiles := GetSecretDirFiles(loadedChartFiles)

	var defaultSecretValuesFiles []*common.BufferedFile
	if !opts.WithoutDefaultSecretValues {
		if defaultSecretValues := GetDefaultSecretValuesFile(loadedChartFiles); defaultSecretValues != nil {
			defaultSecretValuesFiles = append(defaultSecretValuesFiles, defaultSecretValues)
		}
	}

	var customSecretValuesFiles []*common.BufferedFile

	for _, customSecretValuesFileName := range opts.CustomSecretValueFiles {
		f := &common.BufferedFile{Name: customSecretValuesFileName}

		if opts.LoadFromLocalFilesystem {
			data, err := os.ReadFile(customSecretValuesFileName)
			if err != nil {
				return fmt.Errorf("read custom secret values file %q from local filesystem: %w", customSecretValuesFileName, err)
			}
			f.Data = data
		} else {
			data, err := common.ChartFileReader.ReadChartFile(ctx, customSecretValuesFileName)
			if err != nil {
				return fmt.Errorf("read custom secret values file %q: %w", customSecretValuesFileName, err)
			}
			f.Data = data
		}

		customSecretValuesFiles = append(customSecretValuesFiles, f)
	}

	var encoder *secret.YamlEncoder
	if len(secretDirFiles)+len(defaultSecretValuesFiles)+len(customSecretValuesFiles) > 0 {
		enc, err := secretsManager.GetYamlEncoder(ctx, opts.SecretsWorkingDir, opts.NoDecryptSecrets)
		if err != nil {
			return fmt.Errorf("get secrets yaml encoder: %w", err)
		}
		encoder = enc
	}

	if len(secretDirFiles) > 0 {
		data, err := LoadChartSecretDirFilesData(secretDirFiles, encoder)
		if err != nil {
			return fmt.Errorf("load secret files data: %w", err)
		}
		s.decryptedSecretFilesData = data
	}

	if len(defaultSecretValuesFiles) > 0 {
		values, err := LoadChartSecretValueFiles(ctx, defaultSecretValuesFiles, encoder)
		if err != nil {
			return fmt.Errorf("load default secret values file: %w", err)
		}
		s.decryptedDefaultSecretValues = values
	}

	if len(customSecretValuesFiles) > 0 {
		values, err := LoadChartSecretValueFiles(ctx, customSecretValuesFiles, encoder)
		if err != nil {
			return fmt.Errorf("load custom secret values files: %w", err)
		}
		s.decryptedCustomSecretValues = values
	}

	return nil
}

// GetDecryptedCustomSecretValues returns the merged values of the secret values files passed
// explicitly (--secret-values). Callers must not mutate the returned map.
func (s *SecretsRuntimeData) GetDecryptedCustomSecretValues() map[string]interface{} {
	return s.decryptedCustomSecretValues
}

// GetDecryptedDefaultSecretValues returns the values of the chart's own secret-values.yaml.
// Callers must not mutate the returned map.
func (s *SecretsRuntimeData) GetDecryptedDefaultSecretValues() map[string]interface{} {
	return s.decryptedDefaultSecretValues
}

func (s *SecretsRuntimeData) GetDecryptedSecretFilesData() map[string]string {
	return s.decryptedSecretFilesData
}

func (s *SecretsRuntimeData) GetEncodedSecretValues(ctx context.Context, secretsManager *secrets_manager.SecretsManager, secretsWorkingDir string, noDecryptSecrets bool) (map[string]interface{}, error) {
	decryptedSecretValues := chartcommon.MergeMaps(s.decryptedDefaultSecretValues, s.decryptedCustomSecretValues)
	if len(decryptedSecretValues) == 0 {
		return nil, nil
	}

	enc, err := secretsManager.GetYamlEncoder(ctx, secretsWorkingDir, noDecryptSecrets)
	if err != nil {
		return nil, fmt.Errorf("get secrets yaml encoder: %w", err)
	}

	decryptedSecretsData, err := yaml.Marshal(decryptedSecretValues)
	if err != nil {
		return nil, fmt.Errorf("marshal decrypted secrets yaml: %w", err)
	}

	encryptedSecretsData, err := enc.EncryptYamlData(decryptedSecretsData)
	if err != nil {
		return nil, fmt.Errorf("encrypt secrets data: %w", err)
	}

	var encryptedData map[string]interface{}
	if err := yaml.Unmarshal(encryptedSecretsData, &encryptedData); err != nil {
		return nil, fmt.Errorf("unmarshal encrypted secrets data: %w", err)
	}

	return encryptedData, nil
}

func LoadChartSecretValueFiles(ctx context.Context, secretDirFiles []*common.BufferedFile, encoder *secret.YamlEncoder) (map[string]interface{}, error) {
	var res map[string]interface{}

	for _, f := range secretDirFiles {
		decodedData, untypedScalars, err := encoder.DecryptYamlDataWithUntypedScalars(f.Data)
		if err != nil {
			return nil, fmt.Errorf("decode file %q secret data: %w", f.Name, err)
		}

		warnAboutUntypedSecretValues(ctx, f.Name, untypedScalars)

		rawValues := map[string]interface{}{}
		if err := yaml.Unmarshal(decodedData, &rawValues); err != nil {
			return nil, fmt.Errorf("unmarshal secret values file %s: %w", f.Name, err)
		}

		res = chartcommon.MergeMaps(res, rawValues)
	}

	return res, nil
}

// warnAboutUntypedSecretValues points out values encrypted in a format that does not store the
// YAML type: they are always decrypted as strings, which matters only for those that would
// otherwise be read as a number or a boolean.
func warnAboutUntypedSecretValues(ctx context.Context, fileName string, untypedScalars []secret.UntypedScalar) {
	untypedPaths := lo.FilterMap(untypedScalars, func(scalar secret.UntypedScalar, _ int) (string, bool) {
		var value interface{}
		if err := yaml.Unmarshal([]byte(scalar.Value), &value); err != nil {
			return "", false
		}

		switch value.(type) {
		case bool, float64:
			return formatSecretValuePath(scalar.Path), true
		default:
			return "", false
		}
	})

	if len(untypedPaths) == 0 {
		return
	}

	log.Default.Warn(ctx, "Secret values file %q has values encrypted in an old format that does not store YAML types, so they are read as strings: %s. To make them numbers or booleans, re-enter them with `nelm chart secret values-file edit` (`werf helm secret values edit` in werf).", fileName, strings.Join(untypedPaths, ", "))
}

func formatSecretValuePath(path []interface{}) string {
	var result strings.Builder

	for _, element := range path {
		switch element := element.(type) {
		case int:
			fmt.Fprintf(&result, "[%d]", element)
		default:
			key := fmt.Sprint(element)

			switch {
			case !simpleSecretValuePathKeyRegexp.MatchString(key):
				fmt.Fprintf(&result, "[%s]", strconv.Quote(key))
			case result.Len() > 0:
				result.WriteString("." + key)
			default:
				result.WriteString(key)
			}
		}
	}

	return result.String()
}
