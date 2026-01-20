// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package minkapi

import (
	"errors"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
)

var (
	ErrInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ProgramName)
	// ErrStartFailed is a sentinel error indicating that the core failed to start.
	ErrStartFailed = fmt.Errorf(commonerrors.FmtStartFailed, ProgramName)
	// ErrClientFacadesFailed is a sentinel error indicating that client facades creation failed.
	ErrClientFacadesFailed = errors.New("failed to create client facades")
	// ErrServiceFailed is a sentinel error indicating that the core failed.
	ErrServiceFailed = fmt.Errorf("%s core failed", ProgramName)
	// ErrLoadConfigTemplate is a sentinel error indicating that config template loading failed.
	ErrLoadConfigTemplate = errors.New("cannot load config template")
	// ErrExecuteConfigTemplate is a sentinel error indicating that config template execution failed.
	ErrExecuteConfigTemplate = errors.New("cannot execute config template")
	ErrStoreNotFound         = errors.New("store not found")
	ErrCreateObject          = errors.New("cannot create object")
	ErrDeleteObject          = errors.New("cannot delete object")
	ErrListObjects           = errors.New("cannot list objects")

	ErrUpdateObject = errors.New("cannot update object")
	// ErrCreateView is a sentinel error indicating that view creation failed.
	ErrCreateView = errors.New("cannot create view")
)
