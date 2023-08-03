package errors

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
)

func Multierrorf(format string, errs []error, a ...any) error {
	if len(errs) == 1 {
		return fmt.Errorf(fmt.Sprintf(format, a...)+": %w", errs[0])
	}

	return fmt.Errorf(fmt.Sprintf(format, a...)+": %w", multierror.Append(nil, errs...))
}
