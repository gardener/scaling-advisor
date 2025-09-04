// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
)

var (
	ErrInitFailed = fmt.Errorf("%s init failed", ProgramName)
	// ErrStartFailed is a sentinel error indicating that the service failed to start.
	ErrStartFailed         = fmt.Errorf("%s start failed", ProgramName)
	ErrClientFacadesFailed = errors.New("failed to create client facades")
	// ErrServiceFailed is a sentinel error indicating that the service failed.
	ErrServiceFailed = fmt.Errorf("%s service failed", ProgramName)
	// ErrMissingOpt is a sentinel error indicating that one or more required command line options are missing.
	ErrMissingOpt            = errors.New("missing option")
	ErrLoadConfigTemplate    = errors.New("cannot load config template")
	ErrExecuteConfigTemplate = errors.New("cannot execute config template")
	ErrStoreNotFound         = errors.New("store not found")
	ErrCreateObject          = errors.New("cannot create object")
	ErrDeleteObject          = errors.New("cannot delete object")
	ErrListObjects           = errors.New("cannot list objects")

	ErrUpdateObject = errors.New("cannot update object")

	ErrCreateSandbox = errors.New("cannot create sandbox")
)
