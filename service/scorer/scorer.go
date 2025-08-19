package scorer

import (
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/service/api"
	v1 "k8s.io/api/core/v1"
)

var _ api.GetNodeScoreSelector = GetNodeScoreSelector

func GetNodeScoreSelector(scoringStrategy commontypes.NodeScoringStrategy) (api.NodeScoreSelector, error) {
	panic("implement me")
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
	// TODO
}

func (l LeastCost) Compute(args api.NodeScoreArgs) (api.NodeScore, error) {
	// The least-cost strategy generates a score representing the number of resource units scheduled per unit cost.
	// Here, resource unit is an abstraction used to represent and operate upon multiple heterogeneous
	// resource requests.
	// Resource quantities of different resource types are reduced to a representation in terms of resource units
	// based on pre-configured weights.
	var scheduledResources = make(map[v1.ResourceName]resource.Quantity)
	//add resources required by pods scheduled on scaled candidate node
	for _, pod := range args.ScaledAssignment.ScheduledPods {
		for name, quantity := range pod.AggregatedRequests {
			if value, found := scheduledResources[name]; found {
				value.Add(quantity)
				scheduledResources[name] = value
			} else {
				scheduledResources[name] = quantity.DeepCopy()
			}
		}
	}
	//add resources required by pods scheduled on existing nodes
	for _, assignment := range args.Assignments {
		for _, pod := range assignment.ScheduledPods {
			for name, quantity := range pod.AggregatedRequests {
				if value, found := scheduledResources[name]; found {
					value.Add(quantity)
					scheduledResources[name] = value
				} else {
					scheduledResources[name] = quantity.DeepCopy()
				}
			}
		}
	}
	//calculate total scheduledResources in terms of resource units using weights
	var aggregator float64
	for name, quantity := range scheduledResources {
		if weight, found := l.weights[name]; !found {
			return api.NodeScore{}, fmt.Errorf("no weight found for resource %s", name)
		} else {
			aggregator += weight * float64(quantity.Value())
		}
	}
	//divide total scheduledResources by instance price to get score
	//TODO get value for region
	price, err := l.instancePricing.GetPrice("", args.ScaledAssignment.Node.InstanceType)
	if err != nil {
		return api.NodeScore{}, err
	}
	return api.NodeScore{
		SimulationName:  args.SimulationName,
		Placement:       args.Placement,
		UnscheduledPods: args.UnscheduledPods,
		Value:           int(aggregator / price),
	}, nil
}

var _ api.NodeScorer = (*LeastWaste)(nil)

type LeastWaste struct {
	instancePricing api.InstancePricing
	weights         map[v1.ResourceName]float64
	// TODO
}

func (l LeastWaste) Compute(args api.NodeScoreArgs) (api.NodeScore, error) {
	//	Instead of calculating absolute wastage across the cluster, we look at relative wastage as a score.
	//	Relative wastage can be calculated by summing the wastage on the scaled candidate node
	//	and the "negative" waste created as a result of unscheduled pods being scheduled on to existing nodes.
	//  Existing nodes include simulated winner nodes from previous runs.
	var wastage = make(map[v1.ResourceName]resource.Quantity)
	//start with allocatable of scaled candidate node
	for name, quantity := range args.ScaledAssignment.Node.Allocatable {
		wastage[name] = quantity.DeepCopy()
	}
	//subtract current usage to find wastage on scaled candidate node
	for _, pod := range args.ScaledAssignment.ScheduledPods {
		for name, quantity := range pod.AggregatedRequests {
			if value, found := wastage[name]; found {
				value.Sub(quantity)
				wastage[name] = value
			} else {
				return api.NodeScore{}, fmt.Errorf("scaled node does not support resource %s required by pod %s", name, pod.Name)
			}
		}
	}
	//subtract resources scheduled on existing nodes to find relative wastage
	for _, assignment := range args.Assignments {
		for _, pod := range assignment.ScheduledPods {
			for name, quantity := range pod.AggregatedRequests {
				if value, found := wastage[name]; found {
					value.Sub(quantity)
					wastage[name] = value
				} else {
					return api.NodeScore{}, fmt.Errorf("node %s does not support resource %s required by pod %s", assignment.Node.Name, name, pod.Name)
				}
			}
		}
	}
	//calculate single score from wastage using weights
	var aggregator float64
	for name, quantity := range wastage {
		if weight, found := l.weights[name]; !found {
			return api.NodeScore{}, fmt.Errorf("no weight found for resource %s", name)
		} else {
			aggregator += weight * float64(quantity.Value())
		}
	}
	return api.NodeScore{
		SimulationName:  args.SimulationName,
		Placement:       args.Placement,
		UnscheduledPods: args.UnscheduledPods,
		Value:           int(aggregator),
	}, nil
}
