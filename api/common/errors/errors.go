// Package errors holds common sentinel errors and string formats for error message.
package errors

import (
	"errors"
)

var (
	// ErrMissingOpt is a sentinel error indicating that one or more required command line options are missing.
	ErrMissingOpt = errors.New("missing option")
	// ErrInvalidOptVal is a sentinel error indicating that a specific option has an invalid value
	ErrInvalidOptVal = errors.New("invalid option value")
	// ErrUnImplemented indicates that the feature or operation is unimplemented. It possibly maybe be implemented in the future.
	ErrUnImplemented = errors.New("not implemented")
)

var (
	// FmtInitFailed is a error format indicating that the quoted component failed to initialize.
	FmtInitFailed = "%q initialization failed"
	// FmtStartFailed is a error format indicating that the quoted component failed to start.
	FmtStartFailed = "%q start failed"
)
