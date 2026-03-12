package errs

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

func FormatTemplatingError(err error) error {
	if err == nil || !strings.HasPrefix(err.Error(), "template: ") {
		return err
	}

	var errorsMsgs []string
	currentErr := err
	for currentErr != nil {
		unwrapped := errors.Unwrap(currentErr)
		if unwrapped != nil {
			currentErr = unwrapped
		} else {
			currentErr = nil
			continue
		}

		if len(errorsMsgs) > 0 && errorsMsgs[len(errorsMsgs)-1] == unwrapped.Error() {
			continue
		}

		errorsMsgs = append(errorsMsgs, unwrapped.Error())
	}

	var errParts []string
	for i := 0; i < len(errorsMsgs); i++ {
		if i+1 > len(errorsMsgs)-1 {
			errParts = append(errParts, errorsMsgs[i])
		} else {
			errParts = append(errParts, strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(errorsMsgs[i], errorsMsgs[i+1])), ":"))
		}
	}

	var result error
	for i := len(errParts) - 1; i >= 0; i-- {
		if i == len(errParts)-1 {
			result = errors.New(fmt.Sprintf("\n  %s", errParts[i]))
		} else {
			result = errors.Wrap(result, fmt.Sprintf("\n  %s", errParts[i]))
		}
	}

	return result
}
