// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
)

var (
	// ErrInitFailed is a sentinel error indicating that the scaling-advisor service failed to initialize.
	ErrInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ProgramName)
	// ErrStartFailed is a sentinel error indicating that the scaling-advisor service failed to start.
	ErrStartFailed = fmt.Errorf(commonerrors.FmtStartFailed, ProgramName)
)
