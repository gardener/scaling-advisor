package utilization

import (
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	corev1 "k8s.io/api/core/v1"
)

var _ planner.NodeUtilizationCalculator = (*defaultNodeUtilizationCalculator)(nil)

type defaultNodeUtilizationCalculator struct{}

// New returns a new NodeUtilizationCalculator.
func New() planner.NodeUtilizationCalculator {
	return &defaultNodeUtilizationCalculator{}
}

func (c *defaultNodeUtilizationCalculator) GetUtilization(node corev1.Node, pods []corev1.Pod) planner.NodeUtilization {
	allocatable := node.Status.Allocatable
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

	return planner.NodeUtilization{ResourceRatios: resourceRatios}
}
