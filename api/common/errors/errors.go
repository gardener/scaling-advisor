// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
	// ErrUnimplemented indicates that the feature or operation is unimplemented. It possibly maybe be implemented in the future.
	ErrUnimplemented = errors.New("not implemented")
	// ErrUnexpectedType is a sentinel error representing an unexpected type error which should not happen - generally encountered when casting. Use this in lieu of a panic.
	ErrUnexpectedType = errors.New("unexpected type")
	// ErrLoadTemplate is a sentinel error representing a problem loading a template file
	ErrLoadTemplate = errors.New("failed to load template")
	// ErrExecuteTemplate is a sentinel error indicating that template execution failed.
	ErrExecuteTemplate = errors.New("cannot execute template")
)

var (
	// FmtInitFailed is a error format indicating that the quoted component failed to initialize.
	FmtInitFailed = "%q initialization failed"
	// FmtStartFailed is a error format indicating that the quoted component failed to start.
	FmtStartFailed = "%q start failed"
)
