package utilization

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	corev1 "k8s.io/api/core/v1"
)

var _ planner.NodeUtilizationCalculator = (*defaultNodeUtilizationCalculator)(nil)

type defaultNodeUtilizationCalculator struct{}

func New() planner.NodeUtilizationCalculator {
	return &defaultNodeUtilizationCalculator{}
}

func (c *defaultNodeUtilizationCalculator) GetUtilization(ctx context.Context, view minkapi.View, nodeName string) (planner.NodeUtilization, error) {
	nodes, err := view.ListNodes(ctx, nodeName)
	if err != nil {
		return planner.NodeUtilization{}, fmt.Errorf("failed to get node %q: %w", nodeName, err)
	}
	if len(nodes) == 0 {
		return planner.NodeUtilization{}, fmt.Errorf("node %q not found in view", nodeName)
	}
	allocatable := nodes[0].Status.Allocatable

	pods, err := viewutil.ListPodsOfNode(ctx, view, nodeName)
	if err != nil {
		return planner.NodeUtilization{}, err
	}

	totalRequests := make(corev1.ResourceList)
	for i := range pods {
		for resourceName, qty := range podutil.AggregatePodRequests(&pods[i]) {
			current := totalRequests[resourceName]
			current.Add(qty)
			totalRequests[resourceName] = current
		}
	}

	resourceRatios := make(map[corev1.ResourceName]float64, len(allocatable))
	for resourceName, allocatableQty := range allocatable {
		allocatableMillis := allocatableQty.MilliValue()
		if allocatableMillis == 0 {
			continue
		}
		requestedQty := totalRequests[resourceName]
		resourceRatios[resourceName] = float64(requestedQty.MilliValue()) / float64(allocatableMillis)
	}

	return planner.NodeUtilization{ResourceRatios: resourceRatios}, nil
}
