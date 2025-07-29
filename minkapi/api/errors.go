// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
)

var (
	ErrInitFailed    = errors.New(fmt.Sprintf("%s init failed", ProgramName))
	ErrStartFailed   = fmt.Errorf("%s start failed", ProgramName)
	ErrServiceFailed = fmt.Errorf("%s service failed", ProgramName)

	ErrMissingOpt = errors.New("missing option")

	ErrLoadConfigTemplate    = errors.New("cannot load config template")
	ErrExecuteConfigTemplate = errors.New("cannot execute config template")

	ErrStoreNotFound = errors.New("store not found")
	ErrCreateObject  = errors.New("cannot create object")
	ErrDeleteObject  = errors.New("cannot delete object")
	ErrListObjects   = errors.New("cannot list objects")
)
