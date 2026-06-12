// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scalingconstraintutil

import (
	"slices"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
)

// NodePoolToNodeTemplates returns a map of NodePool.Name to the matching NodeTemplates
// from sc.Spec.NodeTemplates, based on each pool's Requirements.
func NodePoolToNodeTemplates(sc *sacorev1alpha1.ScalingConstraint) map[string][]sacorev1alpha1.NodeTemplate {
	result := make(map[string][]sacorev1alpha1.NodeTemplate, len(sc.Spec.NodePools))
	for _, np := range sc.Spec.NodePools {
		for _, nt := range sc.Spec.NodeTemplates {
			if nodeTemplateMatchesRequirements(nt, np.Requirements) {
				result[np.Name] = append(result[np.Name], nt)
			}
		}
	}
	return result
}

func nodeTemplateMatchesRequirements(nt sacorev1alpha1.NodeTemplate, requirements []sacorev1alpha1.NodePoolRequirement) bool {
	for _, req := range requirements {
		if !nodeTemplateMatchesRequirement(nt, req) {
			return false
		}
	}
	return true
}

func nodeTemplateMatchesRequirement(nt sacorev1alpha1.NodeTemplate, req sacorev1alpha1.NodePoolRequirement) bool {
	var val string
	switch req.Key {
	case "node.kubernetes.io/instance-type":
		val = nt.InstanceType
	case "kubernetes.io/arch":
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
	result := make(map[string][]sacorev1alpha1.NodePlacement, len(sc.Spec.NodePools))
	npToNodeTemplate := NodePoolToNodeTemplates(&sc)
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
		result[np.Name] = placements
	}
	return result
}
