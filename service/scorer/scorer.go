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
	//TODO implement me
	panic("implement me")
}

var _ api.NodeScorer = (*LeastWaste)(nil)

type LeastWaste struct {
	instancePricing api.InstancePricing
	weights         map[v1.ResourceName]float64
	// TODO
}

func (l LeastWaste) Compute(args api.NodeScoreArgs) (api.NodeScore, error) {
	//	Instead of calculating absolute wastage across the cluster, we look at relative wastage as a score.
	//	Relative Wastage can be calculated by summing the wastage on the current simulated node
	//	and, the "negative" waste as a result of unscheduled pods being scheduled on to existing nodes.
	// Existing nodes include simulated winner nodes from previous runs.
	var currWastage = make(map[v1.ResourceName]resource.Quantity)
	for name, quantity := range args.ScaledAssignment.Node.Allocatable {
		if value, found := currWastage[name]; found {
			value.Add(quantity)
			currWastage[name] = value
		} else {
			currWastage[name] = quantity.DeepCopy()
		}
	}
	//subtract current usage to find wastage on current node
	for _, pod := range args.ScaledAssignment.ScheduledPods {
		for name, quantity := range pod.AggregatedRequests {
			if value, found := currWastage[name]; found {
				value.Sub(quantity)
				currWastage[name] = value
			} else {
				return api.NodeScore{}, fmt.Errorf("scaled node does not support resource %s required by pod %s", name, pod.Name)
			}
		}
	}
	//subtract resources scheduled on existing nodes from wastage to find relative wastage
	for _, assignment := range args.Assignments {
		for _, pod := range assignment.ScheduledPods {
			for name, quantity := range pod.AggregatedRequests {
				if value, found := currWastage[name]; found {
					value.Sub(quantity)
					currWastage[name] = value
				} else {
					return api.NodeScore{}, fmt.Errorf("node %s does not support resource %s required by pod %s", assignment.Node.Name, name, pod.Name)
				}
			}
		}
	}
	//calculate final relative wastage score using weights for resource types
	var aggregator float64
	for name, quantity := range currWastage {
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
