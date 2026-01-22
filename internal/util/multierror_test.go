package util

import (
	"errors"
	"fmt"
	"testing"
)

func TestMultiError_Empty(t *testing.T) {
	m := &MultiError{}

	if m.HasErrors() {
		t.Error("expected HasErrors() to be false for empty MultiError")
	}

	if m.OrNilIfNoErrs() != nil {
		t.Error("expected OrNilIfNoErrs() to return nil for empty MultiError")
	}

	if m.Error() != "" {
		t.Errorf("expected empty string, got %q", m.Error())
	}
}

func TestMultiError_SingleError(t *testing.T) {
	m := &MultiError{}
	m.Add(errors.New("single error"))

	if !m.HasErrors() {
		t.Error("expected HasErrors() to be true")
	}

	if m.OrNilIfNoErrs() == nil {
		t.Error("expected OrNilIfNoErrs() to return non-nil")
	}

	expected := "single error"
	if m.Error() != expected {
		t.Errorf("expected %q, got %q", expected, m.Error())
	}
}

func TestMultiError_MultipleErrors(t *testing.T) {
	m := &MultiError{}
	m.Add(errors.New("error one"))
	m.Add(errors.New("error two"))
	m.Add(errors.New("error three"))

	expected := `3 errors occurred:
  * error one
  * error two
  * error three`

	if m.Error() != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, m.Error())
	}
}

func TestMultiError_AddNil(t *testing.T) {
	m := &MultiError{}
	m.Add(nil)
	m.Add(errors.New("real error"))
	m.Add(nil)

	if len(m.errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(m.errs))
	}

	expected := "real error"
	if m.Error() != expected {
		t.Errorf("expected %q, got %q", expected, m.Error())
	}
}

func TestMultiError_AddMultiError(t *testing.T) {
	inner := &MultiError{}
	inner.Add(errors.New("inner error"))

	outer := &MultiError{}
	outer.Add(inner)

	expected := "inner error"
	if outer.Error() != expected {
		t.Errorf("expected %q, got %q", expected, outer.Error())
	}
}

func TestMultiError_NestedWithPrefix(t *testing.T) {
	inner := &MultiError{}
	inner.Add(errors.New("field1: invalid"))
	inner.Add(errors.New("field2: required"))

	outer := &MultiError{}
	outer.Add(fmt.Errorf("validate Resource/foo: %w", inner))

	expected := `2 errors occurred:
  * validate Resource/foo: field1: invalid
  * validate Resource/foo: field2: required`

	if outer.Error() != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, outer.Error())
	}
}

func TestMultiError_DeeplyNested(t *testing.T) {
	level3 := &MultiError{}
	level3.Add(errors.New("deep error"))

	level2 := &MultiError{}
	level2.Add(fmt.Errorf("level2: %w", level3))

	level1 := &MultiError{}
	level1.Add(fmt.Errorf("level1: %w", level2))

	expected := "level1: level2: deep error"
	if level1.Error() != expected {
		t.Errorf("expected %q, got %q", expected, level1.Error())
	}
}

func TestMultiError_MixedNestedAndRegular(t *testing.T) {
	inner := &MultiError{}
	inner.Add(errors.New("inner1"))
	inner.Add(errors.New("inner2"))

	outer := &MultiError{}
	outer.Add(errors.New("regular error"))
	outer.Add(fmt.Errorf("wrapped: %w", inner))
	outer.Add(errors.New("another regular"))

	expected := `4 errors occurred:
  * regular error
  * wrapped: inner1
  * wrapped: inner2
  * another regular`

	if outer.Error() != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, outer.Error())
	}
}

func TestMultiError_Unwrap(t *testing.T) {
	m := &MultiError{}
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	m.Add(err1, err2)

	unwrapped := m.Unwrap()
	if len(unwrapped) != 2 {
		t.Errorf("expected 2 errors, got %d", len(unwrapped))
	}

	if unwrapped[0] != err1 || unwrapped[1] != err2 {
		t.Error("unwrapped errors don't match added errors")
	}
}

func TestMultiError_ErrorsIs(t *testing.T) {
	sentinel := errors.New("sentinel")

	m := &MultiError{}
	m.Add(fmt.Errorf("wrapped: %w", sentinel))

	if !errors.Is(m, sentinel) {
		t.Error("expected errors.Is to find sentinel error")
	}
}

type customError struct {
	Code int
}

func (e *customError) Error() string {
	return fmt.Sprintf("custom error: %d", e.Code)
}

func TestMultiError_ErrorsAs(t *testing.T) {
	custom := &customError{Code: 42}

	m := &MultiError{}
	m.Add(fmt.Errorf("wrapped: %w", custom))

	var target *customError
	if !errors.As(m, &target) {
		t.Error("expected errors.As to find custom error")
	}

	if target.Code != 42 {
		t.Errorf("expected Code 42, got %d", target.Code)
	}
}

func TestMultiError_ChainedAdd(t *testing.T) {
	m := &MultiError{}
	m.Add(errors.New("one")).Add(errors.New("two")).Add(errors.New("three"))

	if len(m.errs) != 3 {
		t.Errorf("expected 3 errors, got %d", len(m.errs))
	}
}

func TestMultiError_NestedMultiErrorDirectlyAdded(t *testing.T) {
	inner := &MultiError{}
	inner.Add(errors.New("a"))
	inner.Add(errors.New("b"))

	outer := &MultiError{}
	outer.Add(inner)
	outer.Add(errors.New("c"))

	expected := `3 errors occurred:
  * a
  * b
  * c`

	if outer.Error() != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, outer.Error())
	}
}

func TestMultiError_MultipleWrappersInChain(t *testing.T) {
	inner := &MultiError{}
	inner.Add(errors.New("base error"))

	wrapped := fmt.Errorf("wrapper1: %w", fmt.Errorf("wrapper2: %w", inner))

	outer := &MultiError{}
	outer.Add(wrapped)

	expected := "wrapper1: wrapper2: base error"
	if outer.Error() != expected {
		t.Errorf("expected %q, got %q", expected, outer.Error())
	}
}

func TestMultiError_RealWorldValidationScenario(t *testing.T) {
	resourceErrs := &MultiError{}
	resourceErrs.Add(fmt.Errorf("/spec/replicas: %w", errors.New("expected integer or null, but got string")))
	resourceErrs.Add(fmt.Errorf("/spec/selector/matchLabels/app: %w", errors.New("expected string or null, but got number")))

	validationErrs := &MultiError{}
	validationErrs.Add(fmt.Errorf("validate Deployment/my-app: %w", resourceErrs))
	validationErrs.Add(fmt.Errorf("validate Deployment/my-app2: %w", errors.New("decode: unable to parse quantity's suffix")))

	expected := `3 errors occurred:
  * validate Deployment/my-app: /spec/replicas: expected integer or null, but got string
  * validate Deployment/my-app: /spec/selector/matchLabels/app: expected string or null, but got number
  * validate Deployment/my-app2: decode: unable to parse quantity's suffix`

	if validationErrs.Error() != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, validationErrs.Error())
	}
}
