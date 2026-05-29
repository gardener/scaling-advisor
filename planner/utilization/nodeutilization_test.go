package utilization

import (
	"testing"

	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeNode(allocatable corev1.ResourceList) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status:     corev1.NodeStatus{Allocatable: allocatable},
	}
}

func makePods(requests ...corev1.ResourceList) []corev1.Pod {
	pods := make([]corev1.Pod, 0, len(requests))
	for i, req := range requests {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-" + string(rune('a'+i))},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Resources: corev1.ResourceRequirements{Requests: req}},
				},
			},
		})
	}
	return pods
}

func TestGetUtilization(t *testing.T) {
	calc := New()

	tests := []struct {
		nodeAllocate corev1.ResourceList
		wantRatios   map[corev1.ResourceName]float64
		name         string
		pods         []corev1.ResourceList
	}{
		{
			name: "single pod consumes half of cpu and memory",
			nodeAllocate: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			pods: []corev1.ResourceList{
				{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			wantRatios: map[corev1.ResourceName]float64{
				corev1.ResourceCPU:    0.5,
				corev1.ResourceMemory: 0.5,
			},
		},
		{
			name: "no pods yields zero utilization",
			nodeAllocate: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			pods: nil,
			wantRatios: map[corev1.ResourceName]float64{
				corev1.ResourceCPU:    0.0,
				corev1.ResourceMemory: 0.0,
			},
		},
		{
			name: "multiple pods requests are summed",
			nodeAllocate: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
			pods: []corev1.ResourceList{
				{corev1.ResourceCPU: resource.MustParse("1")},
				{corev1.ResourceCPU: resource.MustParse("1")},
				{corev1.ResourceCPU: resource.MustParse("1")},
			},
			wantRatios: map[corev1.ResourceName]float64{
				corev1.ResourceCPU: 0.75,
			},
		},
		{
			name: "fully utilized node",
			nodeAllocate: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"),
			},
			pods: []corev1.ResourceList{
				{corev1.ResourceCPU: resource.MustParse("2")},
			},
			wantRatios: map[corev1.ResourceName]float64{
				corev1.ResourceCPU: 1.0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := makeNode(tc.nodeAllocate)
			pods := makePods(tc.pods...)

			util := calc.GetUtilization(node, pods)

			assertRatios(t, util, tc.wantRatios)
		})
	}
}

func assertRatios(t *testing.T, util plannerapi.NodeUtilization, wantRatios map[corev1.ResourceName]float64) {
	t.Helper()
	for res, wantRatio := range wantRatios {
		gotRatio, ok := util.ResourceRatios[res]
		if !ok {
			t.Errorf("resource %q missing from ResourceRatios", res)
			continue
		}
		const epsilon = 1e-9
		if diff := gotRatio - wantRatio; diff > epsilon || diff < -epsilon {
			t.Errorf("resource %q: got ratio %v, want %v", res, gotRatio, wantRatio)
		}
	}
}
