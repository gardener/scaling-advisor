package testutils

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func AssertError(t *testing.T, got error, want error) {
	t.Helper()
	if isNil(got) && isNil(want) {
		return
	}
	if errors.Is(got, want) || strings.Contains(got.Error(), want.Error()) {
		t.Logf("Expected error: %v", got)
	} else {
		t.Errorf("Unexpected error, got: %v, want: %v", got, want)
	}
}

// isNil checks if v is nil. (source: https://antonz.org/do-not-testify/)
func isNil(v any) bool {
	if v == nil {
		return true
	}
	// A non-nil interface can still hold a nil value, so we must check the underlying value.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice,
		reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}

func GetFunctionName(t *testing.T, fn any) string {
	t.Helper()
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}
