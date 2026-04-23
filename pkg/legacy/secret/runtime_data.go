package secret

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/common"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
)

var _ RuntimeData = (*SecretsRuntimeData)(nil)

type DecodeAndLoadSecretsOptions = chartcommon.DecodeAndLoadSecretsOptions

type RuntimeData interface {
	DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*common.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error
	GetEncodedSecretValues(ctx context.Context, secretsManager *secrets_manager.SecretsManager, secretsWorkingDir string, noDecryptSecrets bool) (map[string]interface{}, error)
	GetDecryptedSecretValues() map[string]interface{}
	GetDecryptedSecretFilesData() map[string]string
}

type SecretsRuntimeData struct {
	decryptedSecretFilesData map[string]string
	decryptedSecretValues    map[string]interface{}
}

func NewSecretsRuntimeData() *SecretsRuntimeData {
	return &SecretsRuntimeData{
		decryptedSecretFilesData: make(map[string]string),
	}
}

func (s *SecretsRuntimeData) DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*common.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts DecodeAndLoadSecretsOptions) error {
	secretDirFiles := GetSecretDirFiles(loadedChartFiles)

	var loadedSecretValuesFiles []*common.BufferedFile

	if !opts.WithoutDefaultSecretValues {
		if defaultSecretValues := GetDefaultSecretValuesFile(loadedChartFiles); defaultSecretValues != nil {
			loadedSecretValuesFiles = append(loadedSecretValuesFiles, defaultSecretValues)
		}
	}

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

		loadedSecretValuesFiles = append(loadedSecretValuesFiles, f)
	}

	var encoder *secret.YamlEncoder
	if len(secretDirFiles)+len(loadedSecretValuesFiles) > 0 {
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

	if len(loadedSecretValuesFiles) > 0 {
		values, err := LoadChartSecretValueFiles(loadedSecretValuesFiles, encoder)
		if err != nil {
			return fmt.Errorf("load secret value files: %w", err)
		}
		s.decryptedSecretValues = values
	}

	return nil
}

func (s *SecretsRuntimeData) GetDecryptedSecretFilesData() map[string]string {
	return s.decryptedSecretFilesData
}

func (s *SecretsRuntimeData) GetDecryptedSecretValues() map[string]interface{} {
	return s.decryptedSecretValues
}

func (s *SecretsRuntimeData) GetEncodedSecretValues(ctx context.Context, secretsManager *secrets_manager.SecretsManager, secretsWorkingDir string, noDecryptSecrets bool) (map[string]interface{}, error) {
	if len(s.decryptedSecretValues) == 0 {
		return nil, nil
	}

	enc, err := secretsManager.GetYamlEncoder(ctx, secretsWorkingDir, noDecryptSecrets)
	if err != nil {
		return nil, fmt.Errorf("get secrets yaml encoder: %w", err)
	}

	decryptedSecretsData, err := yaml.Marshal(s.decryptedSecretValues)
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

func LoadChartSecretValueFiles(secretDirFiles []*common.BufferedFile, encoder *secret.YamlEncoder) (map[string]interface{}, error) {
	var res map[string]interface{}

	for _, f := range secretDirFiles {
		decodedData, err := encoder.DecryptYamlData(f.Data)
		if err != nil {
			return nil, fmt.Errorf("decode file %q secret data: %w", f.Name, err)
		}

		rawValues := map[string]interface{}{}
		if err := yaml.Unmarshal(decodedData, &rawValues); err != nil {
			return nil, fmt.Errorf("unmarshal secret values file %s: %w", f.Name, err)
		}

		res = common.LegacyCoalesceTablesFunc(rawValues, res)
	}

	return res, nil
}
