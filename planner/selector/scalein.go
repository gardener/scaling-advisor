// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selector

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	plannerapivalidation "github.com/gardener/scaling-advisor/planner/api/validation"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ plannerapi.ScaleInCandidateSelector = (*DefaultScaleInCandidateSelector)(nil)
)

// DefaultScaleInCandidateSelector is the default implementation of [plannerapi.ScaleInCandidateSelector].
type DefaultScaleInCandidateSelector struct {
}

// Select implements [plannerapi.ScaleInCandidateSelector].
//
// It returns all nodes that are both underutilized (all configured thresholds satisfied) and whose
// removal would not violate any [PodDisruptionBudgets].
// Candidates are first sorted by least pool priority then by highest underutilization, and are
// accepted greedily in that order: each accepted node's pods consume PDB budget before the next
// candidate is evaluated.
//
// [PodDisruptionBudgets]: https://kubernetes.io/docs/reference/kubernetes-api/policy/pod-disruption-budget-v1/
func (s *DefaultScaleInCandidateSelector) Select(ctx context.Context, args plannerapi.ScaleInCandidateSelectorArgs) ([]corev1.Node, error) {
	log := logr.FromContextOrDiscard(ctx)

	nodes, err := args.View.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	allPods, err := args.View.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pdbs, err := listPDBs(ctx, args.View)
	if err != nil {
		return nil, fmt.Errorf("failed to list PodDisruptionBudgets: %w", err)
	}

	// bucket pods by node name
	podsByNode := make(map[string][]corev1.Pod, len(nodes))
	for i := range allPods {
		nn := allPods[i].Spec.NodeName
		if nn != "" {
			podsByNode[nn] = append(podsByNode[nn], allPods[i])
		}
	}

	// identify underutilized nodes and compute their utilization score
	type candidate struct {
		node     corev1.Node
		poolName string
		priority int32
		// maxUtil is the highest utilization ratio across all configured threshold resources;
		// lower maxUtil means more underutilized — preferred for scale-in.
		maxUtil float64
	}

	var candidates []candidate
	for _, node := range nodes {
		if err := plannerapivalidation.ValidateNodeLabels(node.Labels); err != nil {
			log.V(4).Info("Skipping node: missing required labels", "node", node.Name, "err", err)
			continue
		}
		poolName := node.Labels[apicommon.LabelNodePoolName]

		// check pool min resource limits: skip if removing this node would drop the pool below its min
		if minLimits, ok := args.PoolMinLimits[poolName]; ok {
			if wouldViolatePoolMin(node, nodes, podsByNode, minLimits) {
				log.V(4).Info("Skipping node: removal would violate pool min resource limits", "node", node.Name, "pool", poolName)
				continue
			}
		}

		utilization := nodeutil.GetNodeUtilization(node, podsByNode[node.Name])

		if !isUnderutilized(utilization, args.UtilizationThresholds) {
			log.V(5).Info("Skipping node: not underutilized", "node", node.Name, "utilization", utilization)
			continue
		}

		priority := args.PoolPriorities[poolName]

		// maxUtil across threshold resources as the sort key (lower = more underutilized)
		maxUtil := 0.0
		for res := range args.UtilizationThresholds {
			if u := utilization[res]; u > maxUtil {
				maxUtil = u
			}
		}

		candidates = append(candidates, candidate{
			node:     node,
			poolName: poolName,
			priority: priority,
			maxUtil:  maxUtil,
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// sort: least priority first, then most underutilized (lowest maxUtil) first
	slices.SortFunc(candidates, func(a, b candidate) int {
		if c := cmp.Compare(a.priority, b.priority); c != 0 {
			return c
		}
		return cmp.Compare(a.maxUtil, b.maxUtil)
	})

	tracker, err := newPDBTracker(pdbs)
	if err != nil {
		return nil, fmt.Errorf("failed to build PDB tracker: %w", err)
	}

	var accepted []corev1.Node
	for _, c := range candidates {
		nodePods := podsByNode[c.node.Name]
		if ok, blockingPod := tracker.CanRemovePods(nodePods); !ok {
			log.V(4).Info("Skipping node: removal would violate PDB", "node", c.node.Name, "blockingPod", blockingPod)
			continue
		}
		tracker.ConsumePods(nodePods)
		accepted = append(accepted, c.node)
	}

	return accepted, nil
}

// isUnderutilized returns true if the given utilization is below all configured thresholds.
func isUnderutilized(utilization map[corev1.ResourceName]float64, thresholds map[corev1.ResourceName]float64) bool {
	for res, threshold := range thresholds {
		if utilization[res] >= threshold {
			return false
		}
	}
	return true
}

// wouldViolatePoolMin returns true if removing node from its pool would leave the pool's total
// allocatable resources below any entry in minLimits.
func wouldViolatePoolMin(node corev1.Node, allNodes []corev1.Node, podsByNode map[string][]corev1.Pod, minLimits corev1.ResourceList) bool {
	poolName := node.Labels[apicommon.LabelNodePoolName]
	poolAllocatable := corev1.ResourceList{}
	for _, n := range allNodes {
		if n.Labels[apicommon.LabelNodePoolName] != poolName || n.Name == node.Name {
			continue
		}
		for res, qty := range n.Status.Allocatable {
			sum := poolAllocatable[res]
			sum.Add(qty)
			poolAllocatable[res] = sum
		}
	}
	_ = podsByNode // reserved for future per-resource used-capacity checks
	for res, minQty := range minLimits {
		avail := poolAllocatable[res]
		if avail.Cmp(minQty) < 0 {
			return true
		}
	}
	return false
}

// listPDBs returns all PodDisruptionBudgets across all namespaces from the given view.
func listPDBs(ctx context.Context, view minkapi.View) ([]policyv1.PodDisruptionBudget, error) {
	facades, err := view.GetClientFacades(ctx, minkapi.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	pdbList, err := facades.Client.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pdbList.Items, nil
}
