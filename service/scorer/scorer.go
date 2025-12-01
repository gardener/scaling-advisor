// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scorer

import (
	"fmt"
	"maps"
	"math"
	"math/rand/v2"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
	corev1 "k8s.io/api/core/v1"
)

var _ service.GetNodeScorer = GetNodeScorer

// GetNodeScorer returns the NodeScorer based on the NodeScoringStrategy
func GetNodeScorer(scoringStrategy commontypes.NodeScoringStrategy, instancePricingAccess service.InstancePricingAccess, resourceWeigher service.ResourceWeigher) (service.NodeScorer, error) {
	switch scoringStrategy {
	case commontypes.NodeScoringStrategyLeastCost:
		return &LeastCost{pricingAccess: instancePricingAccess, resourceWeigher: resourceWeigher}, nil
	case commontypes.NodeScoringStrategyLeastWaste:
		return &LeastWaste{pricingAccess: instancePricingAccess, resourceWeigher: resourceWeigher}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", service.ErrUnsupportedNodeScoringStrategy, scoringStrategy)
	}
}

var _ service.NodeScorer = (*LeastCost)(nil)

// LeastCost contains information required by the least-cost node scoring strategy
type LeastCost struct {
	pricingAccess   service.InstancePricingAccess
	resourceWeigher service.ResourceWeigher
}

// Compute uses the least-cost strategy to generate a score representing the number of normalized resource units (NRU) scheduled per unit cost.
// Here, NRU is an abstraction used to represent and operate upon multiple heterogeneous
// resource requests.
// Resource quantities of different resource types are reduced to a representation in terms of NRU
// based on pre-configured weights.
func (l LeastCost) Compute(args service.NodeScorerArgs) (score service.NodeScore, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: least-cost node scoring failed for simulation %q: %v", service.ErrComputeNodeScore, args.ID, err)
		}
	}()
	//add resources required by pods scheduled on scaled candidate node and existing nodes
	aggregatedPodsResources := getAggregatedScheduledPodsResources(args.ScaledNodePodAssignment, args.OtherNodePodAssignments)
	//calculate total scheduledResources in terms of normalized resource units using weights
	weights, err := l.resourceWeigher.GetWeights(args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	totalNormalizedResourceUnits := getNormalizedResourceUnits(aggregatedPodsResources, weights)
	info, err := l.pricingAccess.GetInfo(args.ScaledNodePlacement.Region, args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	score = service.NodeScore{
		Name:               args.ID,
		Placement:          args.ScaledNodePlacement,
		Value:              int(math.Round(totalNormalizedResourceUnits * 100 / info.HourlyPrice)),
		ScaledNodeResource: args.ScaledNodePodAssignment.NodeResources,
		UnscheduledPods:    args.LeftOverUnscheduledPods,
	}
	return
}

// Select returns the index of the node score for the node with the highest allocatable resources.
// This has been done to bias the scorer to pick larger instance types when all other parameters are the same.
// Larger instance types --> less fragmentation
// if multiple node scores have instance types with the same allocatable, an index is picked at random from them
func (l LeastCost) Select(nodeScores []service.NodeScore) (*service.NodeScore, error) {
	if len(nodeScores) == 0 {
		return nil, service.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		return &nodeScores[0], nil
	}
	var winnerIndices []int
	maxNormalizedAlloc := 0.0
	for index, candidate := range nodeScores {
		weights, err := l.resourceWeigher.GetWeights(candidate.Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		normalizedAlloc := getNormalizedResourceUnits(candidate.ScaledNodeResource.Allocatable, weights)
		if maxNormalizedAlloc == normalizedAlloc {
			winnerIndices = append(winnerIndices, index)
		} else if maxNormalizedAlloc < normalizedAlloc {
			winnerIndices = winnerIndices[:0]
			winnerIndices = append(winnerIndices, index)
			maxNormalizedAlloc = normalizedAlloc
		}
	}
	//pick one winner at random from winnerIndices
	randIndex := rand.IntN(len(winnerIndices)) // #nosec G404 -- cryptographic randomness not required here. It randomly picks one of the node scores with the same least price.
	return &nodeScores[winnerIndices[randIndex]], nil
}

var _ service.NodeScorer = (*LeastWaste)(nil)

// LeastWaste contains information required by the least-waste node scoring strategy
type LeastWaste struct {
	pricingAccess   service.InstancePricingAccess
	resourceWeigher service.ResourceWeigher
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
func (l LeastWaste) Compute(args service.NodeScorerArgs) (nodeScore service.NodeScore, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: least-waste node scoring failed for simulation %q: %v", service.ErrComputeNodeScore, args.ID, err)
		}
	}()
	var wastage = make(map[corev1.ResourceName]int64)
	//start with allocatable of scaled candidate node
	maps.Copy(wastage, args.ScaledNodePodAssignment.NodeResources.Allocatable)
	//subtract resource requests of pods scheduled on scaled node and existing nodes to find delta
	aggregatedPodResources := getAggregatedScheduledPodsResources(args.ScaledNodePodAssignment, args.OtherNodePodAssignments)
	for resourceName, request := range aggregatedPodResources {
		if waste, found := wastage[resourceName]; !found {
			continue
		} else {
			wastage[resourceName] = waste - request
		}
	}
	//calculate single score from wastage using weights
	weights, err := l.resourceWeigher.GetWeights(args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	totalNormalizedResourceUnits := getNormalizedResourceUnits(wastage, weights)
	nodeScore = service.NodeScore{
		Name:               args.ID,
		Placement:          args.ScaledNodePlacement,
		UnscheduledPods:    args.LeftOverUnscheduledPods,
		Value:              int(totalNormalizedResourceUnits * 100),
		ScaledNodeResource: args.ScaledNodePodAssignment.NodeResources,
	}
	return
}

// Select returns the index of the node score for the node with the lowest price.
// if multiple node scores have instance types with the same price, an index is picked at random from them
func (l LeastWaste) Select(nodeScores []service.NodeScore) (*service.NodeScore, error) {
	if len(nodeScores) == 0 {
		return nil, service.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		return &nodeScores[0], nil
	}
	var winnerIndices []int
	leastPrice := math.MaxFloat64
	for index, candidate := range nodeScores {
		info, err := l.pricingAccess.GetInfo(candidate.Placement.Region, candidate.Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		price := info.HourlyPrice
		if leastPrice == price {
			winnerIndices = append(winnerIndices, index)
		} else if leastPrice > price {
			winnerIndices = winnerIndices[:0]
			winnerIndices = append(winnerIndices, index)
			leastPrice = price
		}
	}
	//pick one winner at random from winnerIndices
	randIndex := rand.IntN(len(winnerIndices)) // #nosec G404 -- cryptographic randomness not required here. It randomly picks one of the node scores with the same least price.
	return &nodeScores[randIndex], nil
}

// getNormalizedResourceUnits returns the aggregated sum of the resources in terms of normalized resource units
func getNormalizedResourceUnits(resources map[corev1.ResourceName]int64, weights map[corev1.ResourceName]float64) float64 {
	nru := 0.0
	for resourceName, quantity := range resources {
		if weight, found := weights[resourceName]; !found {
			continue
		} else {
			nru += weight * float64(quantity)
		}
	}
	return nru
}

// getAggregatedScheduledPodsResources returns the sum of the resources requested by pods scheduled due to node scale up. It returns a
// map containing the sums for each resource type
func getAggregatedScheduledPodsResources(scaledNodeAssignments *service.NodePodAssignment, otherAssignments []service.NodePodAssignment) map[corev1.ResourceName]int64 {
	var scheduledResources = make(map[corev1.ResourceName]int64)
	if scaledNodeAssignments != nil {
		//add resources required by pods scheduled on scaled candidate node
		for _, pod := range scaledNodeAssignments.ScheduledPods {
			addPodRequests(pod.AggregatedRequests, scheduledResources)
		}
	}
	//add resources required by pods scheduled on existing nodes
	for _, assignment := range otherAssignments {
		for _, pod := range assignment.ScheduledPods {
			addPodRequests(pod.AggregatedRequests, scheduledResources)
		}
	}
	return scheduledResources
}

// addPodRequests adds the pod's requests to aggregateResources resource-wise
func addPodRequests(podRequest, aggregateResources map[corev1.ResourceName]int64) {
	for resourceName, request := range podRequest {
		if value, ok := aggregateResources[resourceName]; ok {
			aggregateResources[resourceName] = value + request
		} else {
			aggregateResources[resourceName] = request
		}
	}
}
