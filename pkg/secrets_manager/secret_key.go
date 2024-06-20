package secrets_manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/pkg/secret"
)

var WerfHomeDir string

func GenerateSecretKey() ([]byte, error) {
	return secret.GenerateAesSecretKey()
}

func GetRequiredOldSecretKey() ([]byte, error) {
	secretKey := []byte(os.Getenv("WERF_OLD_SECRET_KEY"))
	if len(secretKey) == 0 {
		return nil, fmt.Errorf("WERF_OLD_SECRET_KEY environment required")
	}
	return secretKey, nil
}

func GetRequiredSecretKey(workingDir string) ([]byte, error) {
	var secretKey []byte
	var werfSecretKeyPaths []string
	var notFoundIn []string

	secretKey = []byte(os.Getenv("WERF_SECRET_KEY"))
	if len(secretKey) == 0 {
		notFoundIn = append(notFoundIn, "$WERF_SECRET_KEY")

		var werfSecretKeyPath string

		if workingDir != "" {
			if defaultWerfSecretKeyPath, err := filepath.Abs(filepath.Join(workingDir, ".werf_secret_key")); err != nil {
				return nil, err
			} else {
				werfSecretKeyPaths = append(werfSecretKeyPaths, defaultWerfSecretKeyPath)
			}
		}

		werfSecretKeyPaths = append(werfSecretKeyPaths, filepath.Join(WerfHomeDir, "global_secret_key"))

		for _, path := range werfSecretKeyPaths {
			exist, err := FileExists(path)
			if err != nil {
				return nil, err
			}

			if exist {
				werfSecretKeyPath = path
				break
			} else {
				notFoundIn = append(notFoundIn, path)
			}
		}

		if werfSecretKeyPath != "" {
			data, err := ioutil.ReadFile(werfSecretKeyPath)
			if err != nil {
				return nil, err
			}

			secretKey = []byte(strings.TrimSpace(string(data)))
		}
	}

	if len(secretKey) == 0 {
		return nil, NewEncryptionKeyRequiredError(notFoundIn)
	}

	return secretKey, nil
}

type EncryptionKeyRequiredError struct {
	Msg error
}

func (err *EncryptionKeyRequiredError) Error() string {
	return err.Msg.Error()
}

func NewEncryptionKeyRequiredError(notFoundIn []string) *EncryptionKeyRequiredError {
	notFoundInFormatted := []string{}
	for _, el := range notFoundIn {
		notFoundInFormatted = append(notFoundInFormatted, fmt.Sprintf("%q", el))
	}
	return &EncryptionKeyRequiredError{
		Msg: fmt.Errorf("required encryption key not found in: %s", strings.Join(notFoundInFormatted, ", ")),
	}
}

// FileExists returns true if path exists
func FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func isNotExistError(err error) bool {
	return os.IsNotExist(err) || IsNotADirectoryError(err)
}

func IsNotADirectoryError(err error) bool {
	return strings.HasSuffix(err.Error(), "not a directory")
}
