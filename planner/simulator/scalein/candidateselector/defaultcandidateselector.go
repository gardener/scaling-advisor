package scaleincandidateselector

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/drainutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ planner.ScaleInCandidateSelector = (*defaultScaleInCandidateSelector)(nil)

type defaultScaleInCandidateSelector struct {
	NodeUtilizationCalculator planner.NodeUtilizationCalculator
}

// TODO: accept NodeUtilizationCalculator
func New(nodeUtilizationCalculator planner.NodeUtilizationCalculator) planner.ScaleInCandidateSelector {
	return &defaultScaleInCandidateSelector{
		NodeUtilizationCalculator: nodeUtilizationCalculator,
	}
}

func (s *defaultScaleInCandidateSelector) NextCandidate(ctx context.Context, args planner.ScaleInCandidateArgs, skipNodes *sets.Set[string]) (*corev1.Node, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Get all nodes from the view.
	nodes, err := args.View.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	// Build a pool name -> NodePool lookup from the constraint.
	poolByName := make(map[string]*sacorev1alpha1.NodePool, len(args.Constraint.NodePools))
	for _, pool := range args.Constraint.NodePools {
		poolByName[pool.Name] = &pool
	}

	// Count nodes per pool (before any filtering) for the Min check.
	nodesPerPool := make(map[string]int32)
	for i := range nodes {
		poolName := nodes[i].Labels[commonconstants.LabelNodePoolName]
		nodesPerPool[poolName]++
	}

	// Get all pods for SafeToEvict check.
	allPods, err := args.View.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	// Build a map of node name -> pods assigned to that node.
	podsByNode := make(map[string][]*corev1.Pod)
	for i := range allPods {
		nodeName := allPods[i].Spec.NodeName
		if nodeName != "" {
			podsByNode[nodeName] = append(podsByNode[nodeName], &allPods[i])
		}
	}

	// Filter candidates.
	var candidates []corev1.Node
	for _, node := range nodes {
		nodeName := node.Name
		poolName := node.Labels[commonconstants.LabelNodePoolName]

		// Skip if in skipNodes.
		if skipNodes.Has(nodeName) {
			continue
		}

		// Skip if NodePool.Min has been reached.
		if pool, ok := poolByName[poolName]; ok && pool.Min > 0 {
			if nodesPerPool[poolName] <= pool.Min {
				log.V(5).Info("Skipping node: pool has reached minimum node count",
					"node", nodeName, "pool", poolName, "min", pool.Min, "current", nodesPerPool[poolName])
				skipNodes.Insert(nodeName)
				continue
			}
		}

		// Skip if node is marked with ScaleInDisabledKey.
		if _, disabled := node.Annotations[commonconstants.AnnotationScaleInDisabledKey]; disabled {
			log.V(5).Info("Skipping node: scale-in disabled via annotation", "node", nodeName)
			skipNodes.Insert(nodeName)
			continue
		}

		// Skip if node has pods with `sa.gardener.cloud/safe-to-evict` = "false".
		if drainutil.HasNonEvictablePod(podsByNode[nodeName]) {
			log.V(5).Info("Skipping node: has pods with `sa.gardener.cloud/safe-to-evict=false`", "node", nodeName)
			skipNodes.Insert(nodeName)
			continue
		}

		// Skip if node has pods with disrupted PodDisruptionBudgets.
		nodePods := podsByNode[nodeName]
		if len(nodePods) > 0 {
			if canRemove, blockingPod := args.PDBTracker.CanRemovePods(nodePods); !canRemove {
				log.V(5).Info("Skipping node: has pods with disrupted PodDisruptionBudgets",
					"node", nodeName, "blockingPod", blockingPod.Pod.Name)
				skipNodes.Insert(nodeName)
				continue
			}
		}

		// Skip if node utilization is more than the threshold
		nodeUtilization, err := s.NodeUtilizationCalculator.GetUtilization(ctx, args.View, node.Name)
		if err != nil {
			log.Error(err, "Failed to get node utilization, skipping node", "node", nodeName)
			skipNodes.Insert(nodeName)
			continue
		}

		// TODO: what is watermark and where does it come from?
		watermark := planner.NodeUtilization{
			ResourceRatios: map[corev1.ResourceName]float64{
				corev1.ResourceCPU:    0.5,
				corev1.ResourceMemory: 0.5,
			},
		}
		if !nodeUtilization.BelowUtilizationThreshold(watermark) {
			log.V(5).Info("Skipping node: utilization above threshold", "node", nodeName, "utilization", nodeUtilization.ResourceRatios)
			skipNodes.Insert(nodeName)
			continue
		}

		candidates = append(candidates, node)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Sort candidates by increasing priority (lowest priority first — those are the best scale-in candidates).
	slices.SortFunc(candidates, func(a, b corev1.Node) int {
		pkA := getNodePriorityKey(a, poolByName)
		pkB := getNodePriorityKey(b, poolByName)
		// CmpPriorityKeyDecreasing sorts high-priority first. We negate to get increasing order.
		return -commontypes.CmpPriorityKeyDecreasing(pkA, pkB)
	})

	// Get the sublist of candidates at the lowest priority.
	lowestPK := getNodePriorityKey(candidates[0], poolByName)
	var lowestPriorityCandidates []corev1.Node
	for i := range candidates {
		if getNodePriorityKey(candidates[i], poolByName) != lowestPK {
			break
		}
		lowestPriorityCandidates = append(lowestPriorityCandidates, candidates[i])
	}

	// Randomize the lowest priority candidates.
	// #nosec G404 -- cryptographic randomness not required here
	rand.Shuffle(len(lowestPriorityCandidates), func(i, j int) {
		lowestPriorityCandidates[i], lowestPriorityCandidates[j] = lowestPriorityCandidates[j], lowestPriorityCandidates[i]
	})

	return &lowestPriorityCandidates[0], nil
}

// getNodePriorityKey returns the PriorityKey for a node by looking up its pool and template in the constraint.
func getNodePriorityKey(node corev1.Node, poolByName map[string]*sacorev1alpha1.NodePool) commontypes.PriorityKey {
	poolName := node.Labels[commonconstants.LabelNodePoolName]
	pool, ok := poolByName[poolName]
	if !ok {
		return commontypes.PriorityKey{}
	}
	templateName := node.Labels[commonconstants.LabelNodeTemplateName]
	for _, nt := range pool.NodeTemplates {
		if nt.Name == templateName {
			return commontypes.PriorityKey{First: pool.Priority, Second: nt.Priority}
		}
	}
	return commontypes.PriorityKey{First: pool.Priority}
}
