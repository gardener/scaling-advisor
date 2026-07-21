// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"errors"
)

var (
	// ErrMissingOpt is a sentinel error indicating that one or more required command line options are missing.
	ErrMissingOpt = errors.New("missing option")
	// ErrInvalidOptVal is a sentinel error indicating that a specific option has an invalid value
	ErrInvalidOptVal = errors.New("invalid option value")
	// ErrUnexpectedType is a sentinel error representing an unexpected type error which should not happen - generally encountered when casting. Use this in lieu of a panic.
	ErrUnexpectedType = errors.New("unexpected type")
	// ErrUnsupportedCloudProvider is a sentinel error indicating an unsupported cloud provider was specified.
	ErrUnsupportedCloudProvider = errors.New("unsupported cloud provider")
	// ErrMissingRequiredLabel is a sentinel error indicating that a required label is missing from a resource.
	ErrMissingRequiredLabel = errors.New("missing required label")
	// ErrLoadTemplate is a sentinel error representing a problem loading a template file
	ErrLoadTemplate = errors.New("failed to load template")
	// ErrExecuteTemplate is a sentinel error indicating that template execution failed.
	ErrExecuteTemplate = errors.New("cannot execute template")
	// ErrSerialization is a sentinel error indicating that serialization of an object failed
	ErrSerialization = errors.New("failed to serialize obj")
)

var (
	// FmtInitFailed is an error format indicating that the quoted component failed to initialize.
	FmtInitFailed = "%q initialization failed"
	// FmtStartFailed is a error format indicating that the quoted component failed to start.
	FmtStartFailed = "%q start failed"
)
