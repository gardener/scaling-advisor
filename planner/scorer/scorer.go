// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package scorer

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

var _ plannerapi.GetNodeScorer = GetNodeScorer

// GetNodeScorer returns the NodeScorer based on the NodeScoringStrategy
func GetNodeScorer(scoringStrategy commontypes.NodeScoringStrategy, instancePricingAccess pricingapi.InstancePricingAccess, resourceWeigher plannerapi.ResourceWeigher) (plannerapi.NodeScorer, error) {
	switch scoringStrategy {
	case "":
		return nil, fmt.Errorf("%w: scoring strategy must be specified", plannerapi.ErrCreateNodeScorer)
	case commontypes.NodeScoringStrategyLeastCost:
		return &LeastCost{pricingAccess: instancePricingAccess, resourceWeigher: resourceWeigher}, nil
	case commontypes.NodeScoringStrategyLeastWaste:
		return &LeastWaste{pricingAccess: instancePricingAccess, resourceWeigher: resourceWeigher}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported %q", plannerapi.ErrCreateNodeScorer, scoringStrategy)
	}
}

var _ plannerapi.NodeScorer = (*LeastCost)(nil)

// LeastCost contains information required by the least-cost node scoring strategy
type LeastCost struct {
	pricingAccess   pricingapi.InstancePricingAccess
	resourceWeigher plannerapi.ResourceWeigher
}

// Compute uses the least-cost strategy to generate a score representing the number of normalized resource units (NRU) scheduled per unit cost.
// Here, NRU is an abstraction used to represent and operate upon multiple heterogeneous
// resource requests.
// Resource quantities of different resource types are reduced to a representation in terms of NRU
// based on pre-configured weights.
func (l LeastCost) Compute(ctx context.Context, args plannerapi.NodeScorerArgs) (score plannerapi.NodeScore, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: least-cost node scoring failed for simulation %q: %v", plannerapi.ErrComputeNodeScore, args.ID, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	//add resources required by pods scheduled on scaled candidate node and existing nodes
	aggregatedPodsResources := getAggregatedScheduledPodsResources(args.ScaledNodePodAssignment, args.OtherNodePodAssignments)
	//calculate total scheduledResources in terms of normalized resource units using weights
	weights, err := l.resourceWeigher.GetWeights(args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	totalNormalizedResourceUnits := getNormalizedAggregate(aggregatedPodsResources, weights)
	info, err := l.pricingAccess.GetInfo(args.ScaledNodePlacement.Region, args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	score = plannerapi.NodeScore{
		Name:               args.ID,
		Placement:          args.ScaledNodePlacement,
		ScaledNodeResource: args.ScaledNodePodAssignment.NodeResources,
		UnscheduledPods:    args.LeftOverUnscheduledPods,
	}
	hourlyPrice := info.HourlyPrice
	if hourlyPrice == 0 {
		log.V(2).Info("Instance hourly price is 0, setting score to -1 to avoid division by zero", "templateName", args.ScaledNodePlacement.TemplateName, "instanceType", args.ScaledNodePlacement.InstanceType)
		score.Value = -1
	} else {
		score.Value = int(math.Round(totalNormalizedResourceUnits * 100 / info.HourlyPrice))
	}
	log.V(5).Info("Node score", "value", score.Value, "templateName", args.ScaledNodePlacement.TemplateName, "instanceType", args.ScaledNodePlacement.InstanceType)
	return
}

// Select returns the winning node score. If there are multiple node scores with the same value,
// the node score with the largest allocatable resources among them is returned.
// This has been done to bias the scorer to pick larger instance types when all other parameters are the same.
// Larger instance types --> less fragmentation
// if multiple node scores have instance types with the same allocatable, the winner is picked at random from them
func (l LeastCost) Select(ctx context.Context, nodeScores []plannerapi.NodeScore) (*plannerapi.NodeScore, error) {
	log := logr.FromContextOrDiscard(ctx)
	if len(nodeScores) == 0 {
		return nil, plannerapi.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		log.V(4).Info("Single node score, selected directly", "templateName", nodeScores[0].Placement.TemplateName, "instanceType", nodeScores[0].Placement.InstanceType)
		return &nodeScores[0], nil
	}
	// sort in descending order of node score value
	slices.SortStableFunc(nodeScores, func(a, b plannerapi.NodeScore) int {
		return cmp.Compare(b.Value, a.Value)
	})

	var i int
	weightsCache := make(map[string]map[corev1.ResourceName]float64)
	// find index till which all node scores have max value
	for i = 0; i < len(nodeScores); i++ {
		if nodeScores[i].Value != nodeScores[0].Value { // since max is at index 0
			break
		}
		weights, err := l.resourceWeigher.GetWeights(nodeScores[i].Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		weightsCache[nodeScores[i].Placement.InstanceType] = weights
	}

	var normalizedAllocs = make([]float64, i)
	for index, nodeScore := range nodeScores[:i] {
		normalizedAllocs[index] = getNormalizedAggregate(nodeScore.ScaledNodeResource.Allocatable, weightsCache[nodeScore.Placement.InstanceType])
	}

	// find max normalized Allocatable among nodeScores[:i]
	maxNormalizedAlloc := slices.Max(normalizedAllocs)

	// find indices of node scores with max normalized allocatable among nodeScores[:i]
	var candidateIndices []int
	for index, normalizedAlloc := range normalizedAllocs {
		if normalizedAlloc == maxNormalizedAlloc {
			candidateIndices = append(candidateIndices, index)
		}
	}

	log.V(5).Info("Tie-break by allocatable resources", "numTopValueScores", i, "numMaxAllocScores", len(candidateIndices), "maxNormalizedAlloc", maxNormalizedAlloc)
	//pick one winner at random from winnerIndices
	randIndex := rand.IntN(len(candidateIndices)) // #nosec G404 -- cryptographic randomness not required here. It randomly picks one of the node scores with the same allocatable
	winner := nodeScores[candidateIndices[randIndex]]
	log.V(3).Info("Winner node score", "scoreValue", winner.Value, "templateName", winner.Placement.TemplateName, "instanceType", winner.Placement.InstanceType)
	return &winner, nil
}

var _ plannerapi.NodeScorer = (*LeastWaste)(nil)

// LeastWaste contains information required by the least-waste node scoring strategy
type LeastWaste struct {
	pricingAccess   pricingapi.InstancePricingAccess
	resourceWeigher plannerapi.ResourceWeigher
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
func (l LeastWaste) Compute(ctx context.Context, args plannerapi.NodeScorerArgs) (nodeScore plannerapi.NodeScore, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: least-waste node scoring failed for simulation %q: %v", plannerapi.ErrComputeNodeScore, args.ID, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	var wastage = make(corev1.ResourceList)
	//start with allocatable of scaled candidate node
	maps.Copy(wastage, args.ScaledNodePodAssignment.NodeResources.Allocatable)
	//subtract resource requests of pods scheduled on scaled node and existing nodes to find delta
	aggregatedPodResources := getAggregatedScheduledPodsResources(args.ScaledNodePodAssignment, args.OtherNodePodAssignments)
	for resourceName, request := range aggregatedPodResources {
		if waste, found := wastage[resourceName]; !found {
			continue
		} else {
			waste.Sub(request)
			wastage[resourceName] = waste
		}
	}
	//calculate single score from wastage using weights
	weights, err := l.resourceWeigher.GetWeights(args.ScaledNodePlacement.InstanceType)
	if err != nil {
		return
	}
	totalNormalizedResourceUnits := getNormalizedAggregate(wastage, weights)
	nodeScore = plannerapi.NodeScore{
		Name:               args.ID,
		Placement:          args.ScaledNodePlacement,
		UnscheduledPods:    args.LeftOverUnscheduledPods,
		Value:              int(totalNormalizedResourceUnits * 100),
		ScaledNodeResource: args.ScaledNodePodAssignment.NodeResources,
	}
	log.V(5).Info("Node score", "value", nodeScore.Value, "templateName", args.ScaledNodePlacement.TemplateName, "instanceType", args.ScaledNodePlacement.InstanceType)
	return
}

// Select returns the winning node score with the lowest wastage value. If there are multiple node scores with the same value,
// the node score with the cheapest instance type is returned. If there are multiple node scores with instance types with the same hourly price, a
// node score is selected from them at random and returned
func (l LeastWaste) Select(ctx context.Context, nodeScores []plannerapi.NodeScore) (*plannerapi.NodeScore, error) {
	log := logr.FromContextOrDiscard(ctx)
	if len(nodeScores) == 0 {
		return nil, plannerapi.ErrNoWinningNodeScore
	}
	if len(nodeScores) == 1 {
		log.V(4).Info("Single node score, selected directly", "templateName", nodeScores[0].Placement.TemplateName, "instanceType", nodeScores[0].Placement.InstanceType)
		return &nodeScores[0], nil
	}
	// sort in ascending order of node score value
	slices.SortStableFunc(nodeScores, func(a, b plannerapi.NodeScore) int {
		return cmp.Compare(a.Value, b.Value)
	})
	var i int
	var cachedPrices []float64
	for i = 0; i < len(nodeScores); i++ {
		nodeScore := nodeScores[i]
		if nodeScore.Value != nodeScores[0].Value {
			break
		}
		info, err := l.pricingAccess.GetInfo(nodeScore.Placement.Region, nodeScore.Placement.InstanceType)
		if err != nil {
			return nil, err
		}
		cachedPrices = append(cachedPrices, info.HourlyPrice)
	}
	// find min price among nodeScores[:i]
	minPrice := slices.Min(cachedPrices)
	// find indices of node scores with min price among nodeScores[:i]
	var candidateIndices []int
	for index, price := range cachedPrices {
		if price == minPrice {
			candidateIndices = append(candidateIndices, index)
		}
	}

	log.V(5).Info("Tie-break by cost", "numMinValueScores", i, "numLowestCostScores", len(candidateIndices), "lowestCost", minPrice)
	//pick one winner at random from winnerIndices
	randIndex := rand.IntN(len(candidateIndices)) // #nosec G404 -- cryptographic randomness not required here. It randomly picks one of the node scores with the same least price.
	winner := nodeScores[candidateIndices[randIndex]]
	log.V(3).Info("Winner node score", "scoreValue", winner.Value, "templateName", winner.Placement.TemplateName, "instanceType", winner.Placement.InstanceType)
	return &winner, nil
}

// getNormalizedAggregate returns the aggregated sum of the resources in terms of normalized resource units
func getNormalizedAggregate(resources corev1.ResourceList, weights map[corev1.ResourceName]float64) float64 {
	nru := 0.0
	for resourceName, quantity := range resources {
		if weight, found := weights[resourceName]; !found {
			continue
		} else {
			nru += weight * quantity.AsApproximateFloat64()
		}
	}
	return nru
}

// getAggregatedScheduledPodsResources returns the sum of the resources requested by pods scheduled due to node scale up. It returns a
// map containing the sums for each resource type
func getAggregatedScheduledPodsResources(scaledNodeAssignments *plannerapi.NodePodAssignment, otherAssignments []plannerapi.NodePodAssignment) corev1.ResourceList {
	var scheduledResources = make(corev1.ResourceList)
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
func addPodRequests(podRequest, aggregateResources corev1.ResourceList) {
	for resourceName, request := range podRequest {
		if value, ok := aggregateResources[resourceName]; ok {
			value.Add(request)
			aggregateResources[resourceName] = value
		} else {
			aggregateResources[resourceName] = request.DeepCopy()
		}
	}
}
