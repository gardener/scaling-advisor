// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package poolutil

import (
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
)

// NodePoolToNodeTemplates returns a map of NodePool.Name to the resolved NodeTemplates
// from the given nodeTemplates map, based on each pool's NodeTemplateRefs.
func NodePoolToNodeTemplates(sc *sacorev1alpha1.ScalingConstraintSpec, nodeTemplates map[string]sacorev1alpha1.NodeTemplate) map[string][]sacorev1alpha1.NodeTemplate {
	poolTemplateMap := make(map[string][]sacorev1alpha1.NodeTemplate, len(sc.NodePools))
	for _, np := range sc.NodePools {
		for _, ref := range np.NodeTemplateRefs {
			if nt, ok := nodeTemplates[ref.Name]; ok {
				poolTemplateMap[np.Name] = append(poolTemplateMap[np.Name], nt)
			}
		}
	}
	return poolTemplateMap
}

// GetNodePlacements computes and returns all the possible NodePlacements for this ScalingConstraint,
// using the given nodeTemplates map to resolve NodeTemplateRefs.
func GetNodePlacements(sc sacorev1alpha1.ScalingConstraint, nodeTemplates map[string]sacorev1alpha1.NodeTemplate) map[string][]sacorev1alpha1.NodePlacement {
	poolPlacementMap := make(map[string][]sacorev1alpha1.NodePlacement, len(sc.Spec.NodePools))
	npToNodeTemplate := NodePoolToNodeTemplates(&sc.Spec, nodeTemplates)
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
