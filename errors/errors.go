package errors

import (
	"fmt"
)

func NewValidationError(format string, a ...any) *ValidationError {
	return &ValidationError{
		format: format,
		args:   a,
	}
}

type ValidationError struct {
	format string
	args   []any
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf(e.format, e.args...)
}
