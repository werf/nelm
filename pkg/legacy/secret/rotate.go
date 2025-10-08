package secret

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/common-go/pkg/util"
	"github.com/werf/nelm/pkg/log"
)

func RotateSecretKey(
	ctx context.Context,
	helmChartDir string,
	secretWorkingDir string,
	secretValuesPaths ...string,
) error {
	secretsManager := secrets_manager.Manager

	newEncoder, err := secretsManager.GetYamlEncoder(ctx, secretWorkingDir, false)
	if err != nil {
		return err
	}

	oldEncoder, err := secretsManager.GetYamlEncoderForOldKey(ctx)
	if err != nil {
		return err
	}

	return secretsRegenerate(ctx, newEncoder, oldEncoder, helmChartDir, secretValuesPaths...)
}

func secretsRegenerate(
	ctx context.Context,
	newEncoder, oldEncoder *secret.YamlEncoder,
	helmChartDir string,
	secretValuesPaths ...string,
) error {
	var secretFilesPaths []string
	var secretFilesData map[string][]byte
	var secretValuesFilesData map[string][]byte
	regeneratedFilesData := map[string][]byte{}

	isHelmChartDirExist, err := util.FileExists(helmChartDir)
	if err != nil {
		return err
	}

	if isHelmChartDirExist {
		defaultSecretValuesPath := filepath.Join(helmChartDir, "secret-values.yaml")
		isDefaultSecretValuesExist, err := util.FileExists(defaultSecretValuesPath)
		if err != nil {
			return err
		}

		if isDefaultSecretValuesExist {
			secretValuesPaths = append(secretValuesPaths, defaultSecretValuesPath)
		}

		secretDirectory := filepath.Join(helmChartDir, "secret")
		isSecretDirectoryExist, err := util.FileExists(secretDirectory)
		if err != nil {
			return err
		}

		if isSecretDirectoryExist {
			err = filepath.Walk(secretDirectory,
				func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}

					fileInfo, err := os.Stat(path)
					if err != nil {
						return err
					}

					if !fileInfo.IsDir() {
						secretFilesPaths = append(secretFilesPaths, path)
					}

					return nil
				})
			if err != nil {
				return err
			}
		}
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	secretFilesData, err = readFilesToDecode(secretFilesPaths, pwd)
	if err != nil {
		return err
	}

	secretValuesFilesData, err = readFilesToDecode(secretValuesPaths, pwd)
	if err != nil {
		return err
	}

	if err := regenerateSecrets(ctx, secretFilesData, regeneratedFilesData, oldEncoder.Decrypt, newEncoder.Encrypt); err != nil {
		return err
	}

	if err := regenerateSecrets(ctx, secretValuesFilesData, regeneratedFilesData, oldEncoder.DecryptYamlData, newEncoder.EncryptYamlData); err != nil {
		return err
	}

	for filePath, fileData := range regeneratedFilesData {
		if err := log.Default.InfoBlockErr(ctx, log.BlockOptions{
			BlockTitle: fmt.Sprintf("Saving file %q", filePath),
		}, func() error {
			fileData = append(bytes.TrimSpace(fileData), []byte("\n")...)
			return ioutil.WriteFile(filePath, fileData, 0o644)
		}); err != nil {
			return err
		}
	}

	return nil
}

func regenerateSecrets(
	ctx context.Context,
	filesData, regeneratedFilesData map[string][]byte,
	decodeFunc, encodeFunc func([]byte) ([]byte, error),
) error {
	for filePath, fileData := range filesData {
		if err := log.Default.InfoBlockErr(ctx, log.BlockOptions{
			BlockTitle: fmt.Sprintf("Regenerating file %q", filePath),
		}, func() error {
			data, err := decodeFunc(fileData)
			if err != nil {
				return fmt.Errorf("check old encryption key and file data: %w", err)
			}

			resultData, err := encodeFunc(data)
			if err != nil {
				return err
			}

			regeneratedFilesData[filePath] = resultData

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func readFilesToDecode(filePaths []string, pwd string) (map[string][]byte, error) {
	filesData := map[string][]byte{}
	for _, filePath := range filePaths {
		fileData, err := ioutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}

		if filepath.IsAbs(filePath) {
			filePath, err = filepath.Rel(pwd, filePath)
			if err != nil {
				return nil, err
			}
		}

		filesData[filePath] = bytes.TrimSpace(fileData)
	}

	return filesData, nil
}
