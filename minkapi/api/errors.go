// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
)

var (
	// ErrInitFailed is a sentinel error indicating that the minkapi program failed to initialize.
	ErrInitFailed = fmt.Errorf("%q initialization failed", ProgramName)
	// ErrStartFailed is a sentinel error indicating that the core failed to start.
	ErrStartFailed = fmt.Errorf("%q start failed", ProgramName)
	// ErrClientFacadesFailed is a sentinel error indicating that client facades creation failed.
	ErrClientFacadesFailed = errors.New("failed to create client facades")
	// ErrServiceFailed is a sentinel error indicating that the core failed.
	ErrServiceFailed = fmt.Errorf("%s core failed", ProgramName)
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
	// ErrCreateView is a sentinel error indicating that view creation failed.
	ErrCreateView = errors.New("cannot create view")
)
