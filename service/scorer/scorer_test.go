package scorer

import (
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"slices"
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
			"instance-c-1": 1,
		},
	}
}
func TestLeastWasteScoringStrategy(t *testing.T) {
	weights := map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0}
	instancePricing := NewMockInstancePricing()
	scorer, err := GetNodeScorer(commontypes.LeastWasteNodeScoringStrategy, instancePricing, weights)
	if err != nil {
		t.Fatal(err)
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
			got, err := scorer.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}
}

func TestLeastCostScoringStrategy(t *testing.T) {
	weights := map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0}
	instancePricing := NewMockInstancePricing()
	scorer, err := GetNodeScorer(commontypes.LeastCostNodeScoringStrategy, instancePricing, weights)
	if err != nil {
		t.Fatal(err)
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
			got, err := scorer.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if scoreDiff != "" {
				t.Fatalf("Difference: %s", scoreDiff)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}
}

func TestSelectMaxAllocatable(t *testing.T) {
	weights := map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0}
	instancePricing := NewMockInstancePricing()
	selector, err := GetNodeScoreSelector(commontypes.LeastCostNodeScoringStrategy)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input           []api.NodeScore
		expectedErr     error
		expectedIndexIn []int
	}{
		"single node score": {
			input:           []api.NodeScore{{Name: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr:     nil,
			expectedIndexIn: []int{0},
		},
		"no node score": {
			input:           []api.NodeScore{},
			expectedErr:     nil,
			expectedIndexIn: []int{-1},
		},
		"different allocatables": {
			input: []api.NodeScore{
				{
					Name:               "testing",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 4, 8),
				}},
			expectedErr:     nil,
			expectedIndexIn: []int{1},
		},
		"identical allocatables": {
			input: []api.NodeScore{
				{
					Name:               "testing",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 2, 4),
				},
			},
			expectedErr:     nil,
			expectedIndexIn: []int{0, 1},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			index, err := selector(tc.input, weights, instancePricing)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if !slices.Contains(tc.expectedIndexIn, index) {
				t.Fatalf("Index: %v not in list of expected indices: %v", index, tc.expectedIndexIn)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}

}

func TestSelectMinPrice(t *testing.T) {
	weights := map[v1.ResourceName]float64{v1.ResourceCPU: 5.0, v1.ResourceMemory: 1.0}
	instancePricing := NewMockInstancePricing()
	selector, err := GetNodeScoreSelector(commontypes.LeastWasteNodeScoringStrategy)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input           []api.NodeScore
		expectedErr     error
		expectedIndexIn []int
	}{
		"single node score": {
			input:           []api.NodeScore{{Name: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr:     nil,
			expectedIndexIn: []int{0},
		},
		"no node score": {
			input:           []api.NodeScore{},
			expectedErr:     nil,
			expectedIndexIn: []int{-1},
		},
		"different prices": {
			input: []api.NodeScore{
				{
					Name:               "testing",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 1, 2),
				},
			},
			expectedErr:     nil,
			expectedIndexIn: []int{0},
		},
		"identical prices": {
			input: []api.NodeScore{
				{
					Name:               "testing",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          api.NodePlacementInfo{},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-c-1", 1, 2),
				},
			},
			expectedErr:     nil,
			expectedIndexIn: []int{0, 1},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			index, err := selector(tc.input, weights, instancePricing)
			errDiff := cmp.Diff(tc.expectedErr, err)
			if !slices.Contains(tc.expectedIndexIn, index) {
				t.Fatalf("Index: %v not in list of expected indices: %v", index, tc.expectedIndexIn)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}

}
