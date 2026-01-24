package util

import (
	"errors"
	"fmt"
	"strings"
)

// MultiError collects multiple errors and formats them as a flat list.
// Nested MultiErrors wrapped with fmt.Errorf are automatically flattened,
// with wrapper prefixes prepended to each inner error.
type MultiError struct {
	errs []error
}

// Add appends errors to the collection. Nil errors are ignored.
// Returns the MultiError for chaining.
func (m *MultiError) Add(errs ...error) *MultiError {
	for _, err := range errs {
		if err != nil {
			m.errs = append(m.errs, err)
		}
	}

	return m
}

// Error returns formatted error string. Single error is returned as-is.
// Multiple errors are formatted as "N errors occurred:\n  * err1\n  * err2".
// Nested MultiErrors are flattened with their wrapper chain as prefix.
func (m *MultiError) Error() string {
	if len(m.errs) == 0 {
		return ""
	}

	var flattened []string
	for _, err := range m.errs {
		flattened = append(flattened, flattenErrorWithPrefix(err, "")...)
	}

	if len(flattened) == 1 {
		return flattened[0]
	}

	msg := fmt.Sprintf("%d errors occurred:", len(flattened))
	for _, e := range flattened {
		msg += "\n  * " + e
	}

	return msg
}

func (m *MultiError) Unwrap() []error {
	return m.errs
}

func (m *MultiError) HasErrors() bool {
	return len(m.errs) > 0
}

func (m *MultiError) OrNilIfNoErrs() error {
	if m.HasErrors() {
		return m
	}

	return nil
}

func flattenErrorWithPrefix(err error, prefix string) []string {
	var multi *MultiError
	if errors.As(err, &multi) {
		wrapperPrefix := extractWrapperPrefix(err, multi)

		newPrefix := wrapperPrefix
		if prefix != "" {
			if wrapperPrefix != "" {
				newPrefix = prefix + ": " + wrapperPrefix
			} else {
				newPrefix = prefix
			}
		}

		var result []string
		for _, innerErr := range multi.errs {
			result = append(result, flattenErrorWithPrefix(innerErr, newPrefix)...)
		}

		return result
	}

	errStr := err.Error()
	if prefix != "" {
		return []string{prefix + ": " + errStr}
	}

	return []string{errStr}
}

func extractWrapperPrefix(outer error, inner *MultiError) string {
	var prefixes []string

	current := outer
	for current != nil {
		if m, ok := current.(*MultiError); ok && m == inner {
			break
		}

		unwrapped := errors.Unwrap(current)
		if unwrapped == nil {
			break
		}

		if prefix := extractSingleWrapperPrefix(current, unwrapped); prefix != "" {
			prefixes = append(prefixes, prefix)
		}

		current = unwrapped
	}

	return strings.Join(prefixes, ": ")
}

func extractSingleWrapperPrefix(wrapper, wrapped error) string {
	wrapperStr := wrapper.Error()
	wrappedStr := wrapped.Error()

	if wrapperStr == wrappedStr {
		return ""
	}

	if strings.HasSuffix(wrapperStr, wrappedStr) {
		prefix := wrapperStr[:len(wrapperStr)-len(wrappedStr)]
		return strings.TrimSuffix(prefix, ": ")
	}

	return ""
}
