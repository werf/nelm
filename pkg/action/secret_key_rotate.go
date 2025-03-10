package action

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
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/log"
)

type SecretKeyRotateOptions struct {
	ChartDirPath      string
	LogLevel          log.Level
	NewKey            string
	OldKey            string
	SecretValuesPaths []string
	SecretWorkDir     string
}

func SecretKeyRotate(ctx context.Context, opts SecretKeyRotateOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applySecretKeyRotateOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build secret key rotate options: %w", err)
	}

	if opts.OldKey != "" {
		os.Setenv("WERF_OLD_SECRET_KEY", opts.OldKey)
	}

	if opts.NewKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.NewKey)
	}

	if err := rotateSecretKey(ctx, opts.ChartDirPath, opts.SecretWorkDir, opts.SecretValuesPaths...); err != nil {
		return fmt.Errorf("rotate secret key: %w", err)
	}

	return nil
}

func applySecretKeyRotateOptionsDefaults(opts SecretKeyRotateOptions, currentDir string) (SecretKeyRotateOptions, error) {
	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	if opts.SecretWorkDir == "" {
		var err error
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return SecretKeyRotateOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	return opts, nil
}

func rotateSecretKey(
	ctx context.Context,
	helmChartDir string,
	secretWorkingDir string,
	secretValuesPaths ...string,
) error {
	secretsManager := secrets_manager.Manager

	newEncoder, err := secretsManager.GetYamlEncoder(ctx, secretWorkingDir)
	if err != nil {
		return err
	}

	oldEncoder, err := secretsManager.GetYamlEncoderForOldKey(ctx)
	if err != nil {
		return err
	}

	return secretsRegenerate(newEncoder, oldEncoder, helmChartDir, secretValuesPaths...)
}

func secretsRegenerate(
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

	if err := regenerateSecrets(secretFilesData, regeneratedFilesData, oldEncoder.Decrypt, newEncoder.Encrypt); err != nil {
		return err
	}

	if err := regenerateSecrets(secretValuesFilesData, regeneratedFilesData, oldEncoder.DecryptYamlData, newEncoder.EncryptYamlData); err != nil {
		return err
	}

	for filePath, fileData := range regeneratedFilesData {
		err := logboek.LogProcess(fmt.Sprintf("Saving file %q", filePath)).DoError(func() error {
			fileData = append(bytes.TrimSpace(fileData), []byte("\n")...)
			return ioutil.WriteFile(filePath, fileData, 0o644)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func regenerateSecrets(
	filesData, regeneratedFilesData map[string][]byte,
	decodeFunc, encodeFunc func([]byte) ([]byte, error),
) error {
	for filePath, fileData := range filesData {
		err := logboek.LogProcess(fmt.Sprintf("Regenerating file %q", filePath)).
			DoError(func() error {
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
			})
		if err != nil {
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
