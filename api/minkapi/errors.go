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
	// ErrInitFailed is a sentinel error indicating that the minkapi program failed to initialize.
	ErrInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ProgramName)
	// ErrStartFailed is a sentinel error indicating that the service failed to start.
	ErrStartFailed = fmt.Errorf(commonerrors.FmtStartFailed, ProgramName)
	// ErrClientFacadesFailed is a sentinel error indicating that client facades creation failed.
	ErrClientFacadesFailed = errors.New("failed to create client facades")
	// ErrServiceFailed is a sentinel error indicating that the service failed.
	ErrServiceFailed = fmt.Errorf("%s service failed", ProgramName)
	// ErrLoadConfigTemplate is a sentinel error indicating that config template loading failed.
	ErrLoadConfigTemplate = errors.New("cannot load config template")
	// ErrExecuteConfigTemplate is a sentinel error indicating that config template execution failed.
	ErrExecuteConfigTemplate = errors.New("cannot execute config template")
	// ErrStoreNotFound is a sentinel error indicating that a resource store was not found.
	ErrStoreNotFound = errors.New("store not found")
	// ErrCreateWatcher is a sentinel error indicating that watcher creation failed.
	ErrCreateWatcher = errors.New("cannot create watcher")
	// ErrCreateObject is a sentinel error indicating that object creation failed.
	ErrCreateObject = errors.New("cannot create object")
	// ErrDeleteObject is a sentinel error indicating that object deletion failed.
	ErrDeleteObject = errors.New("cannot delete object")
	// ErrListObjects is a sentinel error indicating that object listing failed.
	ErrListObjects = errors.New("cannot list objects")
	// ErrUpdateObject is a sentinel error indicating that object update failed.
	ErrUpdateObject = errors.New("cannot update object")
	// ErrCreateSandbox is a sentinel error indicating that sandbox creation failed.
	ErrCreateSandbox = errors.New("cannot create sandbox")
)
