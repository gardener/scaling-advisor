// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scalingconstraintutil

import (
	"slices"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// NodePoolToNodeTemplates returns a map of NodePool.Name to the matching NodeTemplates
// from sc.Spec.NodeTemplates, based on each pool's Requirements.
func NodePoolToNodeTemplates(sc *sacorev1alpha1.ScalingConstraintSpec) map[string][]sacorev1alpha1.NodeTemplate {
	poolTemplateMap := make(map[string][]sacorev1alpha1.NodeTemplate, len(sc.NodePools))
	for _, np := range sc.NodePools {
		for _, nt := range sc.NodeTemplates {
			if nodeTemplateMatchesRequirements(nt, np.Requirements) {
				poolTemplateMap[np.Name] = append(poolTemplateMap[np.Name], nt)
			}
		}
	}
	return poolTemplateMap
}

func nodeTemplateMatchesRequirements(nt sacorev1alpha1.NodeTemplate, requirements []sacorev1alpha1.NodePoolRequirement) bool {
	for _, req := range requirements {
		if !NodeTemplateMatchesRequirement(nt, req) {
			return false
		}
	}
	return true
}

// NodeTemplateMatchesRequirement checks if a given NodeTemplate matches a NodePoolRequirement.
func NodeTemplateMatchesRequirement(nt sacorev1alpha1.NodeTemplate, req sacorev1alpha1.NodePoolRequirement) bool {
	var val string
	switch req.Key {
	case corev1.LabelInstanceTypeStable:
		val = nt.InstanceType
	case corev1.LabelArchStable:
		val = nt.Architecture
	default:
		return false
	}

	switch req.Operator {
	case sacorev1alpha1.NodePoolRequirementOpIn:
		return slices.Contains(req.Values, val)
	case sacorev1alpha1.NodePoolRequirementOpNotIn:
		return !slices.Contains(req.Values, val)
	case sacorev1alpha1.NodePoolRequirementOpExists:
		return val != ""
	case sacorev1alpha1.NodePoolRequirementOpDoesNotExist:
		return val == ""
	default:
		return false
	}
}

// GetNodePlacements computes and returns all the possible `NodePlacement`s for this NodePool.
func GetNodePlacements(sc sacorev1alpha1.ScalingConstraint) map[string][]sacorev1alpha1.NodePlacement {
	poolPlacementMap := make(map[string][]sacorev1alpha1.NodePlacement, len(sc.Spec.NodePools))
	npToNodeTemplate := NodePoolToNodeTemplates(&sc.Spec)
	for _, np := range sc.Spec.NodePools {
		placements := make([]sacorev1alpha1.NodePlacement, 0, len(npToNodeTemplate[np.Name])*len(np.AvailabilityZones))
		for _, nt := range npToNodeTemplate[np.Name] {
			for _, az := range np.AvailabilityZones {
				placements = append(placements, sacorev1alpha1.NodePlacement{
					PoolName:         np.Name,
					TemplateName:     nt.Name,
					InstanceType:     nt.InstanceType,
					Region:           np.Region,
					AvailabilityZone: az,
				})
			}
		}
		poolPlacementMap[np.Name] = placements
	}
	return poolPlacementMap
}
