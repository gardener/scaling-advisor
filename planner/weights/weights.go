// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package weights

import (
	"github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
)

var defaultResourceWeights = createDefaultWeights()

// GetDefaultWeightsFn returns a GetResourceWeightsFunc which provides default resource weights	.
func GetDefaultWeightsFn() planner.GetResourceWeightsFunc {
	return func(_ string) (map[corev1.ResourceName]float64, error) {
		return defaultResourceWeights, nil
	}
}

// createDefaultWeights returns default weights.
// TODO: This is invalid. One must give specific weights for different instance families
// TODO: solve the normalized unit weight linear optimization problem
func createDefaultWeights() map[corev1.ResourceName]float64 {
	return map[corev1.ResourceName]float64{
		//corev1.ResourceEphemeralStorage: 1, // TODO: what should be weight for this ?
		corev1.ResourceMemory: 1,
		corev1.ResourceCPU:    9,
		"nvidia.com/gpu":      20,
	}
}
