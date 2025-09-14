// Package errors holds common sentinel errors and error message formats
package errors

import (
	"errors"
)

var (
	// ErrMissingOpt is a sentinel error indicating that one or more required command line options are missing.
	ErrMissingOpt = errors.New("missing option")

	// ErrInvalidOptVal is a sentinel error indicating that a specific option has an invalid value
	ErrInvalidOptVal = errors.New("invalid option value")
)

var (
	// FmtInitFailed is a error format indicating that the quoted component failed to initialize.
	FmtInitFailed = "%q initialization failed"
	// FmtStartFailed is a error format indicating that the quoted component failed to start.
	FmtStartFailed = "%q start failed"
)
