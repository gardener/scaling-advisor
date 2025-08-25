package scorer

import (
	"fmt"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

type MockInstancePricing struct {
	prices map[string]float64 // key format: "region-instanceType"
}

func (m *MockInstancePricing) GetPrice(region, instanceType string) (float64, error) {
	if price, exists := m.prices[instanceType]; exists {
		return price, nil
	}
	return 0.0, fmt.Errorf("price not found for region %s, instanceType %s", region, instanceType)
}

func CreateMockNode(name, instanceType string, cpu, memory int64) api.NodeResourceInfo {
	return api.NodeResourceInfo{
		Name:         name,
		InstanceType: instanceType,
		Allocatable: map[v1.ResourceName]int64{
			v1.ResourceCPU:    cpu,
			v1.ResourceMemory: memory,
		},
	}
}

func CreateMockPod(name string, cpu, memory int64) api.PodResourceInfo {
	return api.PodResourceInfo{
		UID: "pod-12345",
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: "default",
		},
		AggregatedRequests: map[v1.ResourceName]int64{
			v1.ResourceCPU:    cpu,
			v1.ResourceMemory: memory,
		},
	}
}

// Helper function to create mock with predefined prices
func NewMockInstancePricing() *MockInstancePricing {
	return &MockInstancePricing{
		prices: map[string]float64{
			"instance-a-1": 1,
			"instance-a-2": 2,
			"instance-b-1": 4,
			"instance-b-2": 8,
		},
	}
}
func TestLeastWasteScoringStrategy(t *testing.T) {
	l := LeastWaste{
		instancePricing: NewMockInstancePricing(),
		weights:         map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0},
	}
	assignment := api.NodePodAssignment{
		Node: CreateMockNode("simNode1", "instance-a-1", 2, 4),
		ScheduledPods: []api.PodResourceInfo{
			CreateMockPod("simPodA", 1, 2),
		},
	}
	tests := map[string]struct {
		input         api.NodeScoreArgs
		expectedErr   error
		expectedScore api.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: api.NodeScoreArgs{
				Name:             "testing",
				Placement:        api.NodePlacementInfo{},
				ScaledAssignment: &assignment,
				Assignments:      nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				Name:               "testing",
				Placement:          api.NodePlacementInfo{},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: api.NodeScoreArgs{
				Name:             "testing",
				Placement:        api.NodePlacementInfo{},
				ScaledAssignment: &assignment,
				Assignments: []api.NodePodAssignment{{
					Node:          CreateMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []api.PodResourceInfo{CreateMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				Name:               "testing",
				Placement:          api.NodePlacementInfo{},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignment.Node,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := l.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
		})
	}
}

func TestLeastCostScoringStrategy(t *testing.T) {
	l := LeastCost{
		instancePricing: NewMockInstancePricing(),
		weights:         map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0},
	}
	assignment := api.NodePodAssignment{
		Node: CreateMockNode("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []api.PodResourceInfo{
			CreateMockPod("simPodA", 1, 2),
		},
	}
	tests := map[string]struct {
		input         api.NodeScoreArgs
		expectedErr   error
		expectedScore api.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: api.NodeScoreArgs{
				Name:             "testing",
				Placement:        api.NodePlacementInfo{Region: ""},
				ScaledAssignment: &assignment,
				Assignments:      nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				Name:               "testing",
				Placement:          api.NodePlacementInfo{},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: api.NodeScoreArgs{
				Name:             "testing",
				Placement:        api.NodePlacementInfo{Region: ""},
				ScaledAssignment: &assignment,
				Assignments: []api.NodePodAssignment{{
					Node:          CreateMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []api.PodResourceInfo{CreateMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				Name:               "testing",
				Placement:          api.NodePlacementInfo{},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.Node,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := l.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
		})
	}
}
