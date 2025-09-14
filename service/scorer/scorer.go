// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scorer

import (
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
	corev1 "k8s.io/api/core/v1"
	"math"
	"math/rand/v2"
)

// getAggregatedScheduledPodsResources returns the sum of the resources requested by pods scheduled due to node scale up. It returns a
// map containing the sums for each resource type
func getAggregatedScheduledPodsResources(scaledNodeAssignments *service.NodePodAssignment, otherAssignments []service.NodePodAssignment) (scheduledResources map[corev1.ResourceName]int64) {
	scheduledResources = make(map[corev1.ResourceName]int64)
	//add resources required by pods scheduled on scaled candidate node
	for _, pod := range scaledNodeAssignments.ScheduledPods {
		for resourceName, request := range pod.AggregatedRequests {
			if value, ok := scheduledResources[resourceName]; ok {
				scheduledResources[resourceName] = value + request
			} else {
				scheduledResources[resourceName] = request
			}
		}
	}
	//add resources required by pods scheduled on existing nodes
	for _, assignment := range otherAssignments {
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
	return scheduledResources
}

var _ service.GetNodeScorer = GetNodeScorer

func GetNodeScorer(scoringStrategy commontypes.NodeScoringStrategy, instancePricingAccess service.InstancePricingAccess, weightsFn service.GetWeightsFunc) (service.NodeScorer, error) {
	switch scoringStrategy {
	case commontypes.LeastCostNodeScoringStrategy:
		return &LeastCost{instancePricingAccess: instancePricingAccess, weightsFn: weightsFn}, nil
	case commontypes.LeastWasteNodeScoringStrategy:
		return &LeastWaste{instancePricing: instancePricingAccess, weightsFn: weightsFn}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", service.ErrUnsupportedNodeScoringStrategy, scoringStrategy)
	}
}

var _ service.NodeScorer = (*LeastCost)(nil)

type LeastCost struct {
	instancePricingAccess service.InstancePricingAccess
	weightsFn             service.GetWeightsFunc
}

// Compute uses the least-cost strategy to generate a score representing the number of resource units scheduled per unit cost.
// Here, resource unit is an abstraction used to represent and operate upon multiple heterogeneous
// resource requests.
// Resource quantities of different resource types are reduced to a representation in terms of resource units
// based on pre-configured weights.
func (l LeastCost) Compute(args service.NodeScoreArgs) (score service.NodeScore, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: least-cost node scoring failed for simulation %q: %v", service.ErrComputeNodeScore, args.ID, err)
		}
	}()
	//add resources required by pods scheduled on scaled candidate node and existing nodes
	aggregatedPodsResources := getAggregatedScheduledPodsResources(args.ScaledAssignment, args.OtherAssignments)
	//calculate total scheduledResources in terms of normalized resource units using weights
	var totalNormalizedResourceUnits float64
	weights, err := l.weightsFn(args.Placement.InstanceType)
	for resourceName, quantity := range aggregatedPodsResources {
		if weight, found := weights[resourceName]; !found {
			continue
		} else {
			totalNormalizedResourceUnits += weight * float64(quantity)
		}
	}
	//divide total NormalizedResourceUnits by instance price to get score
	info, err := l.instancePricingAccess.GetInfo(args.Placement.Region, args.Placement.InstanceType)
	if err != nil {
		return service.NodeScore{}, err
	}
	return service.NodeScore{
		ID:                 args.ID,
		Placement:          args.Placement,
		Value:              int(math.Round(totalNormalizedResourceUnits * 100 / info.HourlyPrice)),
		ScaledNodeResource: args.ScaledAssignment.Node,
		UnscheduledPods:    args.UnscheduledPods,
	}, nil
}

var _ service.NodeScorer = (*LeastWaste)(nil)

type LeastWaste struct {
	instancePricing service.InstancePricingAccess
	weightsFn       service.GetWeightsFunc
}

// Compute returns the NodeScore for the least-waste strategy. Instead of calculating absolute wastage across the cluster,
// we look at delta wastage as a score.
// Delta wastage can be calculated by summing the wastage on the scaled candidate node
// and the "negative" waste created as a result of unscheduled pods being scheduled on to existing nodes.
// Existing nodes include simulated winner nodes from previous runs.
// Waste = Alloc(ScaledNode) - TotalResourceRequests(Pods scheduled due to scale up)
// Example:
// SN* - simulated node
// N* - existing node
// Case 1: pods assigned to scaled node only
// SN1: 4GB allocatable
// Pod A : 1 GB --> SN1
// Pod B:  2 GB --> SN1
// Pod C: 1 GB --> SN1
//
// Waste = 4-(1+2+1) = 0
//
// Case 2: pods assigned to existing nodes also
// SN2: 4GB
// N2: 8GB avail
// N3: 4GB avail
// Pod A : 1 GB --> SN1
// Pod B:  2 GB --> N2
// Pod C: 3 GB --> N3
//
// Waste = 4 - (1+2+3) = -2
func (l LeastWaste) Compute(args service.NodeScoreArgs) (nodeScore service.NodeScore, err error) {
	var wastage = make(map[corev1.ResourceName]int64)
	//start with allocatable of scaled candidate node
	for resourceName, quantity := range args.ScaledAssignment.Node.Allocatable {
		wastage[resourceName] = quantity
	}
	//subtract resource requests of pods scheduled on scaled node and existing nodes to find delta
	aggregatedPodResources := getAggregatedScheduledPodsResources(args.ScaledAssignment, args.OtherAssignments)
	for resourceName, request := range aggregatedPodResources {
		if waste, found := wastage[resourceName]; found {
			wastage[resourceName] = waste - request
		} else {
			continue
		}
	}
	//calculate single score from wastage using weights
	weights, err := l.weightsFn(args.Placement.InstanceType)
	var totalNormalizedResourceUnits float64
	for resourceName, waste := range wastage {
		if weight, found := weights[resourceName]; !found {
			return nodeScore, fmt.Errorf("no weight found for resourceName %s", resourceName)
		} else {
			totalNormalizedResourceUnits += weight * float64(waste)
		}
	}
	nodeScore = service.NodeScore{
		ID:                 args.ID,
		Placement:          args.Placement,
		UnscheduledPods:    args.UnscheduledPods,
		Value:              int(totalNormalizedResourceUnits * 100),
		ScaledNodeResource: args.ScaledAssignment.Node,
	}
	return nodeScore, nil
}

var _ service.GetNodeScoreSelector = GetNodeScoreSelector

// GetNodeScoreSelector returns the NodeScoreSelector based on the scoring strategy
func GetNodeScoreSelector(scoringStrategy commontypes.NodeScoringStrategy) (service.NodeScoreSelector, error) {
	switch scoringStrategy {
	case commontypes.LeastCostNodeScoringStrategy:
		return SelectMaxAllocatable, nil
	case commontypes.LeastWasteNodeScoringStrategy:
		return SelectMinPrice, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", service.ErrUnsupportedNodeScoringStrategy, scoringStrategy)
	}
}

// SelectMaxAllocatable returns the index of the node score for the node with the highest allocatable resources.
// This has been done to bias the scorer to pick larger instance types when all other parameters are the same.
// Larger instance types --> less fragmentation
// if multiple node scores have instance types with the same allocatable, an index is picked at random from them
func SelectMaxAllocatable(nodeScores []service.NodeScore, weightsFn service.GetWeightsFunc, pricing service.InstancePricingAccess) (winner *service.NodeScore, err error) {
	if len(nodeScores) == 0 {
		return nil, service.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		return &nodeScores[0], nil
	}
	var winners []int
	var maxNormalizedAlloc float64
	weights, err := weightsFn(nodeScores[0].Placement.InstanceType)
	if err != nil {
		return nil, err
	}
	for resourceName, quantity := range nodeScores[0].ScaledNodeResource.Allocatable {
		if weight, ok := weights[resourceName]; ok {
			maxNormalizedAlloc += weight * float64(quantity)
		} else {
			continue
		}
	}
	winners = append(winners, 0)
	for index, candidate := range nodeScores[1:] {
		var normalizedAlloc float64
		weights, err = weightsFn(candidate.Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		for resourceName, quantity := range candidate.ScaledNodeResource.Allocatable {
			if weight, ok := weights[resourceName]; ok {
				normalizedAlloc += weight * float64(quantity)
			} else {
				continue
			}
		}
		if maxNormalizedAlloc == normalizedAlloc {
			winners = append(winners, index+1)
		} else if maxNormalizedAlloc < normalizedAlloc {
			winners = winners[:0]
			winners = append(winners, index+1)
			maxNormalizedAlloc = normalizedAlloc
		}
	}
	//pick one winner at random from winners
	return &nodeScores[winners[rand.IntN(len(winners))]], nil
}

// SelectMinPrice returns the index of the node score for the node with the lowest price.
// if multiple node scores have instance types with the same price, an index is picked at random from them
func SelectMinPrice(nodeScores []service.NodeScore, weightsFn service.GetWeightsFunc, pricing service.InstancePricingAccess) (winner *service.NodeScore, err error) {
	if len(nodeScores) == 0 {
		return nil, service.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		return &nodeScores[0], nil
	}
	var winners []int
	info, err := pricing.GetInfo(nodeScores[0].Placement.Region, nodeScores[0].Placement.InstanceType)
	if err != nil {
		return nil, err
	}
	leastPrice := info.HourlyPrice
	winners = append(winners, 0)
	for index, candidate := range nodeScores[1:] {
		info, err := pricing.GetInfo(candidate.Placement.Region, candidate.Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		price := info.HourlyPrice
		if leastPrice == price {
			winners = append(winners, index+1)
		} else if leastPrice > price {
			winners = winners[:0]
			winners = append(winners, index+1)
			leastPrice = price
		}
	}
	//pick one winner at random from winners
	return &nodeScores[rand.IntN(len(winners))], nil
}
