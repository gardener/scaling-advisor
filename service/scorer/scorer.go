package scorer

import (
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/service/api"
	v1 "k8s.io/api/core/v1"
	"math/rand/v2"
)

var _ api.GetNodeScoreSelector = GetNodeScoreSelector

func GetNodeScoreSelector(scoringStrategy commontypes.NodeScoringStrategy) (api.NodeScoreSelector, error) {
	switch scoringStrategy {
	case commontypes.LeastCostNodeScoringStrategy:
		return SelectMaxAllocatable, nil
	case commontypes.LeastWasteNodeScoringStrategy:
		return SelectMinPrice, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", api.ErrUnsupportedNodeScoringStrategy, scoringStrategy)
	}
}

// SelectMaxAllocatable returns the index of the node score for the node with the highest allocatable.
// This has been done to bias the scorer to pick larger instance types when all other parameters are the same.
// Larger instance types --> less fragmentation
// if multiple node scores have instance types with the same allocatable, an index is picked at random from them
var SelectMaxAllocatable api.NodeScoreSelector = func(nodeScores []api.NodeScore, weights map[v1.ResourceName]int64, pricing api.InstancePricing) (int, error) {
	if len(nodeScores) == 0 {
		return -1, nil
	}
	if len(nodeScores) == 1 {
		return 0, nil
	}
	var winners []int
	var maxNormalizedAlloc int64
	for resourceName, quantity := range nodeScores[0].ScaledNodeResource.Allocatable {
		if weight, ok := weights[resourceName]; ok {
			maxNormalizedAlloc += weight * quantity
		} else {
			return -1, fmt.Errorf("no weight found for resource %s", resourceName)
		}
	}
	winners = append(winners, 0)
	for index, candidate := range nodeScores[1:] {
		var normalizedAlloc int64
		for resourceName, quantity := range candidate.ScaledNodeResource.Allocatable {
			if weight, ok := weights[resourceName]; ok {
				normalizedAlloc += weight * quantity
			} else {
				return -1, fmt.Errorf("no weight found for resource %s", resourceName)
			}
		}
		if maxNormalizedAlloc == normalizedAlloc {
			winners = append(winners, index)

		} else if maxNormalizedAlloc < normalizedAlloc {
			winners = winners[:0]
			winners = append(winners, index)
			maxNormalizedAlloc = normalizedAlloc
		}
	}
	//pick one winner at random from winners
	return rand.IntN(len(winners)), nil
}

// SelectMinPrice returns the index of the node score for the node with the lowest price.
// if multiple node scores have instance types with the same price, an index is picked at random from them
var SelectMinPrice api.NodeScoreSelector = func(nodeScores []api.NodeScore, weights map[v1.ResourceName]int64, pricing api.InstancePricing) (int, error) {
	if len(nodeScores) == 0 {
		return -1, nil
	}
	if len(nodeScores) == 1 {
		return 0, nil
	}
	var winners []int
	leastPrice, err := pricing.GetPrice(nodeScores[0].Placement.Region, nodeScores[0].ScaledNodeResource.InstanceType)
	winners = append(winners, 0)
	if err != nil {
		return -1, err
	}
	for index, candidate := range nodeScores[1:] {
		price, err := pricing.GetPrice(candidate.Placement.Region, candidate.ScaledNodeResource.InstanceType)
		if err != nil {
			return -1, err
		}
		if leastPrice == price {
			winners = append(winners, index)
		} else if leastPrice > price {
			winners = winners[:0]
			winners = append(winners, index)
			leastPrice = price
		}
	}
	//pick one winner at random from winners
	return rand.IntN(len(winners)), nil
}

var _ api.GetNodeScorer = GetNodeScorer

func GetNodeScorer(scoringStrategy commontypes.NodeScoringStrategy, instancePricing api.InstancePricing, weights map[v1.ResourceName]float64) (api.NodeScorer, error) {
	switch scoringStrategy {
	case commontypes.LeastCostNodeScoringStrategy:
		return &LeastCost{instancePricing: instancePricing, weights: weights}, nil
	case commontypes.LeastWasteNodeScoringStrategy:
		return &LeastWaste{instancePricing: instancePricing, weights: weights}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", api.ErrUnsupportedNodeScoringStrategy, scoringStrategy)
	}
}

var _ api.NodeScorer = (*LeastCost)(nil)

type LeastCost struct {
	instancePricing api.InstancePricing
	weights         map[v1.ResourceName]float64
}

func (l LeastCost) Compute(args api.NodeScoreArgs) (api.NodeScore, error) {
	// The least-cost strategy generates a score representing the number of resource units scheduled per unit cost.
	// Here, resource unit is an abstraction used to represent and operate upon multiple heterogeneous
	// resource requests.
	// Resource quantities of different resource types are reduced to a representation in terms of resource units
	// based on pre-configured weights.
	var scheduledResources = make(map[v1.ResourceName]int64)
	//add resources required by pods scheduled on scaled candidate node
	for _, pod := range args.ScaledAssignment.ScheduledPods {
		for resourceName, request := range pod.AggregatedRequests {
			if value, found := scheduledResources[resourceName]; found {
				scheduledResources[resourceName] = value + request
			} else {
				scheduledResources[resourceName] = request
			}
		}
	}
	//add resources required by pods scheduled on existing nodes
	for _, assignment := range args.Assignments {
		for _, pod := range assignment.ScheduledPods {
			for resourceName, request := range pod.AggregatedRequests {
				if value, found := scheduledResources[resourceName]; found {
					scheduledResources[resourceName] = value + request
				} else {
					scheduledResources[resourceName] = request
				}
			}
		}
	}
	//calculate total scheduledResources in terms of resource units using weights
	var aggregator float64
	for resourceName, quantity := range scheduledResources {
		if weight, found := l.weights[resourceName]; !found {
			return api.NodeScore{}, fmt.Errorf("no weight found for resource %s", resourceName)
		} else {
			aggregator += weight * float64(quantity)
		}
	}
	//divide total scheduledResources by instance price to get score
	price, err := l.instancePricing.GetPrice(args.Placement.Region, args.ScaledAssignment.Node.InstanceType)
	if err != nil {
		return api.NodeScore{}, err
	}
	return api.NodeScore{
		SimulationName:     args.SimulationName,
		Placement:          args.Placement,
		UnscheduledPods:    args.UnscheduledPods,
		Value:              int(aggregator / price),
		ScaledNodeResource: args.ScaledAssignment.Node,
	}, nil
}

var _ api.NodeScorer = (*LeastWaste)(nil)

type LeastWaste struct {
	instancePricing api.InstancePricing
	weights         map[v1.ResourceName]float64
}

func (l LeastWaste) Compute(args api.NodeScoreArgs) (api.NodeScore, error) {
	//	Instead of calculating absolute wastage across the cluster, we look at relative wastage as a score.
	//	Relative wastage can be calculated by summing the wastage on the scaled candidate node
	//	and the "negative" waste created as a result of unscheduled pods being scheduled on to existing nodes.
	//  Existing nodes include simulated winner nodes from previous runs.
	var wastage = make(map[v1.ResourceName]int64)
	//start with allocatable of scaled candidate node
	for resourceName, quantity := range args.ScaledAssignment.Node.Allocatable {
		wastage[resourceName] = quantity
	}
	//subtract current usage to find wastage on scaled candidate node
	for _, pod := range args.ScaledAssignment.ScheduledPods {
		for resourceName, request := range pod.AggregatedRequests {
			if waste, found := wastage[resourceName]; found {
				wastage[resourceName] = waste - request
			} else {
				return api.NodeScore{}, fmt.Errorf("scaled node does not support resourceName %s required by pod %s", resourceName, pod.Name)
			}
		}
	}
	//subtract resources scheduled on existing nodes to find relative wastage
	for _, assignment := range args.Assignments {
		for _, pod := range assignment.ScheduledPods {
			for resourceName, request := range pod.AggregatedRequests {
				if waste, found := wastage[resourceName]; found {
					wastage[resourceName] = waste - request
				} else {
					return api.NodeScore{}, fmt.Errorf("node %s does not support resourceName %s required by pod %s", assignment.Node.Name, resourceName, pod.Name)
				}
			}
		}
	}
	//calculate single score from wastage using weights
	var aggregator float64
	for resourceName, waste := range wastage {
		if weight, found := l.weights[resourceName]; !found {
			return api.NodeScore{}, fmt.Errorf("no weight found for resourceName %s", resourceName)
		} else {
			aggregator += weight * float64(waste)
		}
	}
	return api.NodeScore{
		SimulationName:     args.SimulationName,
		Placement:          args.Placement,
		UnscheduledPods:    args.UnscheduledPods,
		Value:              int(aggregator),
		ScaledNodeResource: args.ScaledAssignment.Node,
	}, nil
}
