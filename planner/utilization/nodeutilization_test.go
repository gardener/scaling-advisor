package utilization

import (
	"context"
	"errors"
	"testing"

	scaleintestutil "github.com/gardener/scaling-advisor/planner/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetUtilization(t *testing.T) {
	calc := New()

	tests := []struct {
		name         string
		nodeAllocate corev1.ResourceList
		pods         []corev1.ResourceList
		wantRatios   map[corev1.ResourceName]float64
		wantErr      bool
		lookupNode   string
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
		{
			name:       "node not found returns error",
			lookupNode: "ghost-node",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := scaleintestutil.NewTestView(t)
			nodeName := "test-node"
			if tc.lookupNode != "" {
				nodeName = tc.lookupNode
			} else {
				scaleintestutil.AddNode(t, v, nodeName, scaleintestutil.NodeOpts{Allocatable: tc.nodeAllocate})
				for i, req := range tc.pods {
					scaleintestutil.AddPod(t, v, "pod-"+string(rune('a'+i)), "default", nodeName, scaleintestutil.PodOpts{Requests: req})
				}
			}

			util, err := calc.GetUtilization(context.Background(), v, nodeName)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for resource, wantRatio := range tc.wantRatios {
				gotRatio, ok := util.ResourceRatios[resource]
				if !ok {
					t.Errorf("resource %q missing from ResourceRatios", resource)
					continue
				}
				const epsilon = 1e-9
				if diff := gotRatio - wantRatio; diff > epsilon || diff < -epsilon {
					t.Errorf("resource %q: got ratio %v, want %v", resource, gotRatio, wantRatio)
				}
			}
		})
	}
}

func TestGetUtilizationListNodesError(t *testing.T) {
	calc := New()
	v := &scaleintestutil.FailingView{ListNodesErr: errors.New("node listing failed")}
	_, err := calc.GetUtilization(context.Background(), v, "any-node")
	if err == nil {
		t.Fatal("expected error from ListNodes failure, got nil")
	}
}

func TestGetUtilizationListPodsError(t *testing.T) {
	calc := New()
	v := &scaleintestutil.FailingView{ListPodsErr: errors.New("pod listing failed")}
	_, err := calc.GetUtilization(context.Background(), v, "any-node")
	if err == nil {
		t.Fatal("expected error from ListPods failure, got nil")
	}
}
