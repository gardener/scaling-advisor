// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package scalingconstraintutil

import (
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
)

// NodePoolToNodeTemplates returns a map of NodePool.Name to the resolved NodeTemplates
// from the given NodeTemplateSetSpec, based on each pool's NodeTemplateRefs.
func NodePoolToNodeTemplates(sc *sacorev1alpha1.ScalingConstraintSpec, nts *sacorev1alpha1.NodeTemplateSetSpec) map[string][]sacorev1alpha1.NodeTemplate {
	byName := make(map[string]sacorev1alpha1.NodeTemplate, len(nts.Templates))
	for _, nt := range nts.Templates {
		byName[nt.Name] = nt
	}
	poolTemplateMap := make(map[string][]sacorev1alpha1.NodeTemplate, len(sc.NodePools))
	for _, np := range sc.NodePools {
		for _, ref := range np.NodeTemplateRefs {
			if nt, ok := byName[ref.Name]; ok {
				poolTemplateMap[np.Name] = append(poolTemplateMap[np.Name], nt)
			}
		}
	}
	return poolTemplateMap
}

// GetNodePlacements computes and returns all the possible NodePlacements for this ScalingConstraint.
func GetNodePlacements(sc sacorev1alpha1.ScalingConstraint, nts sacorev1alpha1.NodeTemplateSetSpec) map[string][]sacorev1alpha1.NodePlacement {
	poolPlacementMap := make(map[string][]sacorev1alpha1.NodePlacement, len(sc.Spec.NodePools))
	npToNodeTemplate := NodePoolToNodeTemplates(&sc.Spec, &nts)
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
