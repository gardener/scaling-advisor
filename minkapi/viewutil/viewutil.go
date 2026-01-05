package viewutil

import (
	"context"

	"github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/api/core/v1"
)

// ListUnscheduledPods returns all Pods from the given View that are not scheduled to any Node.
func ListUnscheduledPods(ctx context.Context, view minkapi.View) ([]corev1.Pod, error) {
	allPods, err := view.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, err
	}
	unscheduledPods := make([]corev1.Pod, 0, len(allPods))
	for _, p := range allPods {
		if p.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, p)
		}
	}
	return unscheduledPods, nil
}
