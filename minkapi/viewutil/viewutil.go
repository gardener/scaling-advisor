// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package viewutil

import (
	"context"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/podutil"
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
func LogNodeAndPodNames(ctx context.Context, prefix string, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return err
	}
	allNodes, err := view.ListNodes(ctx)
	if err != nil {
		return err
	}
	log.V(2).Info(prefix+"|count of nodes and pods",
		"viewName", view.GetName(),
		"totalNodes", len(allNodes),
		"totalPods", len(allPods),
		"totalUnscheduledPods", podutil.CountUnscheduledPods(allPods))
	if logutil.VerbosityFromContext(ctx) > 2 {
		for idx, pod := range allPods {
			log.V(3).Info(prefix+"|pod in view",
				"viewName", view.GetName(), "idx", idx, "podName", pod.Name, "podNamespace", pod.Namespace,
				"assignedNodeName", pod.Spec.NodeName, "podRequests", pod.Spec.Containers[0].Resources.Requests)
		}
		for _, node := range allNodes {
			log.V(3).Info(prefix+"|node in view",
				"viewName", view.GetName(),
				"nodeName", node.Name,
				"nodePool", node.Labels[commonconstants.LabelNodePoolName],
				"instanceType", node.Labels[corev1.LabelInstanceTypeStable],
				"region", node.Labels[corev1.LabelTopologyRegion],
				"zone", node.Labels[corev1.LabelTopologyZone],
				"allocatable", node.Status.Allocatable)
		}
	}
	return nil
}
