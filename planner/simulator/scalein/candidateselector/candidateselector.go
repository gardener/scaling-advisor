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
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

var _ plannerapi.ScaleInCandidateSelector = (*scaleInCandidateSelector)(nil)

type nodeAndPriority struct {
	node     corev1.Node
	priority commontypes.Priority
}

type scaleInCandidateSelector struct {
	NodeUtilizationCalculator     plannerapi.NodeUtilizationCalculator
	candidateNodesWithPriorityMap map[string]nodeAndPriority
}

// New returns an instance of plannerapi.NodeUtilizationCalculator.
func New(nodeUtilizationCalculator plannerapi.NodeUtilizationCalculator) plannerapi.ScaleInCandidateSelector {
	return &scaleInCandidateSelector{
		NodeUtilizationCalculator: nodeUtilizationCalculator,
	}
}

func (s *scaleInCandidateSelector) Init(ctx context.Context, args plannerapi.ScaleInCandidateSelectorArgs) error {
	log := logr.FromContextOrDiscard(ctx)

	// Get all nodes from the view.
	nodes, err := args.View.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	// Get all pods for SafeToEvict check.
	allPods, err := args.View.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
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

	// Build a map of node name -> pods assigned to that node.
	podsByNode := make(map[string][]corev1.Pod)
	for i := range allPods {
		nodeName := allPods[i].Spec.NodeName
		if nodeName != "" {
			podsByNode[nodeName] = append(podsByNode[nodeName], allPods[i])
		}
	}

	candidateNodesWithPriorityMap := make(map[string]nodeAndPriority)
	for _, node := range nodes {
		nodeName := node.Name
		poolName := node.Labels[commonconstants.LabelNodePoolName]

		// Skip if NodePool.Min has been reached.
		if pool, ok := poolByName[poolName]; ok && pool.Min > 0 {
			if nodesPerPool[poolName] <= pool.Min {
				log.V(5).Info("Skipping node: pool has reached minimum node count",
					"node", nodeName, "pool", poolName, "min", pool.Min, "current", nodesPerPool[poolName], "requestID", ctx.Value("requestID"), "correlationID", ctx.Value("correlationID"))
				continue
			}
		}

		// Skip if node is marked with ScaleInDisabledKey.
		if _, disabled := node.Annotations[commonconstants.AnnotationScaleInDisabled]; disabled {
			log.V(5).Info("Skipping node: scale-in disabled via annotation",
				"node", nodeName, "annotation", commonconstants.AnnotationScaleInDisabled, "requestID", ctx.Value("requestID"), "correlationID", ctx.Value("correlationID"))
			continue
		}

		// Skip if node has pods with `sa.gardener.cloud/safe-to-evict` = "false".
		if podutil.HasNonEvictablePod(podsByNode[nodeName]) {
			log.V(5).Info("Skipping node: has pods with annotation set to false",
				"node", nodeName, "annotation", commonconstants.AnnotationSafeToEvict, "requestID", ctx.Value("requestID"), "correlationID", ctx.Value("correlationID"))
			continue
		}

		candidateNodesWithPriorityMap[nodeName] = nodeAndPriority{priority: getNodePriority(node, poolByName), node: node}
	}

	s.candidateNodesWithPriorityMap = candidateNodesWithPriorityMap

	return nil
}

func (s *scaleInCandidateSelector) NextCandidate(ctx context.Context, args plannerapi.ScaleInCandidateSelectorArgs) (*corev1.Node, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Get all pods for SafeToEvict check.
	allPods, err := args.View.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	// Build a map of node name -> pods assigned to that node.
	podsByNode := make(map[string][]corev1.Pod)
	for i := range allPods {
		nodeName := allPods[i].Spec.NodeName
		if nodeName != "" {
			podsByNode[nodeName] = append(podsByNode[nodeName], allPods[i])
		}
	}

	// Filter candidates.
	var candidatesWithPriority []nodeAndPriority
	for nodeName, nodeWithPriority := range s.candidateNodesWithPriorityMap {
		// Skip if node has pods with disrupted PodDisruptionBudgets.
		nodePods := podsByNode[nodeName]
		if len(nodePods) > 0 {
			if canRemove, blockingPodName := args.PDBTracker.CanRemovePods(nodePods); !canRemove {
				log.V(5).Info("Skipping node: has pods with disrupted PodDisruptionBudgets",
					"node", nodeName, "blockingPod", blockingPodName, "requestID", ctx.Value("requestID"), "correlationID", ctx.Value("correlationID"))
				continue
			}
		}

		// Skip if node utilization is more than the threshold
		nodeUtilization := s.NodeUtilizationCalculator.GetUtilization(nodeWithPriority.node, podsByNode[nodeName])
		if !nodeUtilization.BelowUtilizationThreshold(plannerapi.NodeUtilization{ResourceRatios: args.UtilizationThresholds}) {
			log.V(5).Info("Skipping node: utilization above threshold",
				"node", nodeName, "utilization", nodeUtilization.ResourceRatios, "requestID", ctx.Value("requestID"), "correlationID", ctx.Value("correlationID"))
			continue
		}

		candidatesWithPriority = append(candidatesWithPriority, nodeWithPriority)
	}

	if len(candidatesWithPriority) == 0 {
		return nil, nil
	}

	slices.SortFunc(candidatesWithPriority, func(a, b nodeAndPriority) int {
		return commontypes.CmpPriorityIncreasing(a.priority, b.priority)
	})

	var lowestPriorityCandidates []corev1.Node
	for _, nodeWithPriority := range candidatesWithPriority {
		if nodeWithPriority.priority != candidatesWithPriority[0].priority {
			break
		}
		lowestPriorityCandidates = append(lowestPriorityCandidates, nodeWithPriority.node)
	}

	log.V(5).Info("Lowest priority candidate nodes for scale-in", "nodes", lowestPriorityCandidates)

	return &lowestPriorityCandidates[rand.IntN(len(lowestPriorityCandidates))], nil // #nosec G404 -- cryptographic randomness not required here. It randomly picks one of the equally-prioritized scale-in candidate nodes.
}

func (s *scaleInCandidateSelector) RemoveCandidateNode(nodeName string) {
	delete(s.candidateNodesWithPriorityMap, nodeName)
}

// getNodePriority returns the Priority for a node by looking up its pool and template in the constraint.
func getNodePriority(node corev1.Node, poolByName map[string]*sacorev1alpha1.NodePool) commontypes.Priority {
	poolName := node.Labels[commonconstants.LabelNodePoolName]
	pool, ok := poolByName[poolName]
	if !ok {
		return commontypes.Priority{}
	}
	templateName := node.Labels[commonconstants.LabelNodeTemplateName]
	for _, nt := range pool.NodeTemplates {
		if nt.Name == templateName {
			return commontypes.Priority{First: pool.Priority, Second: nt.Priority}
		}
	}
	return commontypes.Priority{First: pool.Priority}
}
