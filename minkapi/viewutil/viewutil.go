// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package viewutil

import (
	"context"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// ListUnscheduledPods returns all Pods from the given View that are not scheduled to any Node.
func ListUnscheduledPods(ctx context.Context, view minkapi.View) ([]corev1.Pod, error) {
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, err
	}
	unscheduledPods := make([]corev1.Pod, 0, len(allPods))
	for _, p := range allPods {
		if p.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, p)
		}
	}
	return unscheduledPods, nil
}

// LogNodeAndPodNames logs the node and pod names in the given minkapi view using logger from the given context if any.
func LogNodeAndPodNames(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return err
	}
	allNodes, err := view.ListNodes(ctx)
	if err != nil {
		return err
	}
	log.Info("Count nodes and pods in the view", "viewName", view.GetName(), "totalPods", len(allPods), "totalNodes", len(allNodes))
	for idx, pod := range allPods {
		log.Info("pod in view", "viewName", view.GetName(), "idx", idx, "podName", pod.Name, "podNamespace", pod.Namespace, "assignedNodeName", pod.Spec.NodeName)
	}
	for _, node := range allNodes {
		log.Info("node in view",
			"viewName", view.GetName(), "nodeName", node.Name, "nodePool", node.Labels[commonconstants.LabelNodePoolName],
			"region", node.Labels[corev1.LabelTopologyRegion], "zone", node.Labels[corev1.LabelTopologyZone])
	}
	return nil
}
