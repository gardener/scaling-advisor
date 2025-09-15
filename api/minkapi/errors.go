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
	// ErrStartFailed is a sentinel error indicating that the service failed to start.
	ErrStartFailed         = fmt.Errorf(commonerrors.FmtStartFailed, ProgramName)
	ErrClientFacadesFailed = errors.New("failed to create client facades")
	// ErrServiceFailed is a sentinel error indicating that the service failed.
	ErrServiceFailed         = fmt.Errorf("%s service failed", ProgramName)
	ErrLoadConfigTemplate    = errors.New("cannot load config template")
	ErrExecuteConfigTemplate = errors.New("cannot execute config template")
	ErrStoreNotFound         = errors.New("store not found")
	ErrCreateWatcher         = errors.New("cannot create watcher")
	ErrCreateObject          = errors.New("cannot create object")
	ErrDeleteObject          = errors.New("cannot delete object")
	ErrListObjects           = errors.New("cannot list objects")

	ErrUpdateObject = errors.New("cannot update object")

	ErrCreateSandbox = errors.New("cannot create sandbox")
)
