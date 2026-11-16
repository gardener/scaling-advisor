// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package validation provides validation functions for planner API types.
package validation

import (
	"fmt"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"

	apicommon "github.com/gardener/scaling-advisor/api/common"
)

// ValidateNodeLabels validates that all labels required by the planner are present in the given label map.
// Returns an error wrapping [apicommon.ErrMissingRequiredLabel] if any required label is absent.
func ValidateNodeLabels(labels map[string]string) error {
	for _, labelName := range plannerapi.RequiredNodeLabelNames.UnsortedList() {
		if _, found := labels[labelName]; !found {
			return fmt.Errorf("%w: missing %q", apicommon.ErrMissingRequiredLabel, labelName)
		}
	}
	return nil
}
