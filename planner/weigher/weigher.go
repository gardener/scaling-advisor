// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package weigher

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
)

var _ plannerapi.ResourceWeigher = (*defaultResourceWeigher)(nil)

// New returns the default instance of ResourceWeigher.
func New() plannerapi.ResourceWeigher {
	return &defaultResourceWeigher{
		weights: createDefaultWeights(),
	}
}

// GetWeights the resource weights for the given instanceType. Currently, this ignores instanceType parameter but this
// needs to be enhanced.
func (w *defaultResourceWeigher) GetWeights(_ string) (map[corev1.ResourceName]float64, error) {
	return w.weights, nil
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

type defaultResourceWeigher struct {
	weights map[corev1.ResourceName]float64
}
