package pricing

import "errors"

var (
	// ErrLoadProviderInstanceTypeInfo is a sentinel error indicating that provider instance type information could not be loaded.
	ErrLoadProviderInstanceTypeInfo = errors.New("cannot load provider instance type info")
)
