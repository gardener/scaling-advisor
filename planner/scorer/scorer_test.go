// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scorer

import (
	"errors"
	"reflect"
	"testing"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLeastWasteScoringStrategy(t *testing.T) {
	access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
	if err != nil {
		t.Fatal(err)
		return
	}
	assignment := planner.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4),
		ScheduledPods: []planner.PodResourceInfo{
			createPodResourceInfo("simPodA", 1, 2),
		},
	}
	//test case where weights are not defined for all resources
	podWithStorage := createPodResourceInfo("simStorage", 2, 4)
	podWithStorage.AggregatedRequests["Storage"] = 10
	assignmentWithStorage := planner.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []planner.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         planner.NodeScorerArgs
		access        planner.InstancePricingAccess
		weightsFn     planner.GetResourceWeightsFunc
		expectedErr   error
		expectedScore planner.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: []planner.NodePodAssignment{{
					NodeResources: createNodeResourceInfo("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []planner.PodResourceInfo{createPodResourceInfo("simPodB", 1, 2)},
				}},
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		"weights undefined for resource type": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignmentWithStorage,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignmentWithStorage.NodeResources,
			},
		},
		"weights function returns an error": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access: access,
			weightsFn: func(_ string) (map[corev1.ResourceName]float64, error) {
				return nil, errors.New("testing error")
			},
			expectedErr:   planner.ErrComputeNodeScore,
			expectedScore: planner.NodeScore{},
		},
		"pricingAccess.GetInfo() function returns an error": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access: &testInfoAccess{err: errors.New("testing error")},
			weightsFn: func(_ string) (map[corev1.ResourceName]float64, error) {
				return nil, errors.New("testing error")
			},
			expectedErr:   planner.ErrComputeNodeScore,
			expectedScore: planner.NodeScore{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastWaste, tc.access, tc.weightsFn)
			if err != nil {
				t.Fatal(err)
				return
			}
			got, err := scorer.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err, cmpopts.EquateErrors())
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
	access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
	if err != nil {
		t.Fatal(err)
		return
	}
	assignment := planner.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []planner.PodResourceInfo{
			createPodResourceInfo("simPodA", 1, 2),
		},
	}
	//test case where weights are not defined for all resources
	podWithStorage := createPodResourceInfo("simStorage", 1, 2)
	podWithStorage.AggregatedRequests["Storage"] = 10
	assignmentWithStorage := planner.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []planner.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         planner.NodeScorerArgs
		access        planner.InstancePricingAccess
		weightsFn     planner.GetResourceWeightsFunc
		expectedErr   error
		expectedScore planner.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: []planner.NodePodAssignment{{
					NodeResources: createNodeResourceInfo("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []planner.PodResourceInfo{createPodResourceInfo("simPodB", 1, 2)},
				}},
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		"weights undefined for resource type": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignmentWithStorage,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weightsFn:   testWeightsFunc,
			expectedErr: nil,
			expectedScore: planner.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignmentWithStorage.NodeResources,
			},
		}, "weights function returns an error": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access: access,
			weightsFn: func(_ string) (map[corev1.ResourceName]float64, error) {
				return nil, errors.New("testing error")
			},
			expectedErr:   planner.ErrComputeNodeScore,
			expectedScore: planner.NodeScore{},
		},
		"pricingAccess.GetInfo() function returns an error": {
			input: planner.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access: &testInfoAccess{err: errors.New("testing error")},
			weightsFn: func(_ string) (map[corev1.ResourceName]float64, error) {
				return nil, errors.New("testing error")
			},
			expectedErr:   planner.ErrComputeNodeScore,
			expectedScore: planner.NodeScore{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, tc.access, tc.weightsFn)
			if err != nil {
				t.Fatal(err)
			}
			got, err := scorer.Compute(tc.input)
			scoreDiff := cmp.Diff(tc.expectedScore, got)
			errDiff := cmp.Diff(tc.expectedErr, err, cmpopts.EquateErrors())
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
	access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, access, planner.GetResourceWeightsFunc(testWeightsFunc))
	if err != nil {
		t.Fatal(err)
	}
	simNodeWithStorage := createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)
	simNodeWithStorage.Allocatable["Storage"] = 10
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []planner.NodeScore
		expectedErr error
		expectedIn  []planner.NodeScore
	}{
		"single node score": {
			input:       []planner.NodeScore{{Name: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []planner.NodeScore{{Name: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []planner.NodeScore{},
			expectedErr: planner.ErrNoWinningNodeScore,
			expectedIn:  []planner.NodeScore{},
		},
		"different allocatables": {
			input: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", 4, 8),
				}},
			expectedErr: nil,
			expectedIn: []planner.NodeScore{{
				Name:               "testing2",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              1,
				ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", 4, 8),
			}},
		},
		"identical allocatables": {
			input: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", 2, 4),
				},
			},
			expectedErr: nil,
			expectedIn: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", 2, 4),
				},
			},
		},
		"undefined weights for resource type": {
			input: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 4, 8)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: simNodeWithStorage,
				}},
			expectedErr: nil,
			expectedIn: []planner.NodeScore{{
				Name:               "testing1",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
				UnscheduledPods:    nil,
				Value:              1,
				ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 4, 8),
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := scorer.Select(tc.input)
			errDiff := cmp.Diff(tc.expectedErr, err, cmpopts.EquateErrors())
			found := false
			if winningNodeScore == nil && len(tc.expectedIn) == 0 {
				found = true
			} else {
				for _, expectedNodeScore := range tc.expectedIn {
					if cmp.Equal(*winningNodeScore, expectedNodeScore) {
						found = true
						break
					}
				}
			}
			if found == false {
				t.Fatalf("Winning Node Score not returned. Expected winning node score to be in: %v, got: %v", tc.expectedIn, winningNodeScore)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}
}

func TestSelectMinPrice(t *testing.T) {
	access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, access, planner.GetResourceWeightsFunc(testWeightsFunc))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []planner.NodeScore
		expectedErr error
		expectedIn  []planner.NodeScore
	}{
		"single node score": {
			input:       []planner.NodeScore{{Name: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []planner.NodeScore{{Name: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []planner.NodeScore{},
			expectedErr: planner.ErrNoWinningNodeScore,
			expectedIn:  []planner.NodeScore{},
		},
		"different prices": {
			input: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)}},
		},
		"identical prices": {
			input: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []planner.NodeScore{
				{
					Name:               "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", 2, 4)},
				{
					Name:               "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", 1, 2),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := scorer.Select(tc.input)
			errDiff := cmp.Diff(tc.expectedErr, err, cmpopts.EquateErrors())
			found := false
			if winningNodeScore == nil && len(tc.expectedIn) == 0 {
				found = true
			} else {
				for _, expectedNodeScore := range tc.expectedIn {
					if cmp.Equal(*winningNodeScore, expectedNodeScore) {
						found = true
						break
					}
				}
			}
			if found == false {
				t.Fatalf("Winning NodeResources Score not returned. Expected winning node score to be in: %v, got: %v", tc.expectedIn, winningNodeScore)
			}
			if errDiff != "" {
				t.Fatalf("Difference: %s", errDiff)
			}
		})
	}
}

func TestGetNodeScorer(t *testing.T) {
	tests := map[string]struct {
		expectedError error
		input         commontypes.NodeScoringStrategy
		expectedType  string
	}{
		"least-cost strategy": {
			input:         commontypes.NodeScoringStrategyLeastCost,
			expectedType:  "*scorer.LeastCost",
			expectedError: nil,
		},
		"least-waste strategy": {
			input:         commontypes.NodeScoringStrategyLeastWaste,
			expectedType:  "*scorer.LeastWaste",
			expectedError: nil,
		},
		"invalid strategy": {
			input:         "invalid",
			expectedType:  "",
			expectedError: planner.ErrCreateNodeScorer,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
			if err != nil {
				t.Fatalf("GetInstancePricingAccessWithFakeData failed with error: %v", err)
			}
			got, err := GetNodeScorer(tc.input, access, planner.GetResourceWeightsFunc(testWeightsFunc))
			if tc.expectedError == nil {
				if err != nil {
					t.Fatalf("Expected error to be nil but got %v", err)
				}
			} else if tc.expectedError != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Fatalf("Expected error to wrap %v but got %v", tc.expectedError, err)
				} else if err == nil {
					t.Fatalf("Expected error to be %v but got nil", tc.expectedError)
				}
			}
			if tc.expectedType != "" {
				if got == nil {
					t.Fatalf("Expected scorer to be %s but got nil", tc.expectedType)
				} else {
					gotType := reflect.TypeOf(got).String()
					if gotType != tc.expectedType {
						t.Fatalf("Expected type %s but got %s", tc.expectedType, gotType)
					}
				}
			}
		})
	}
}

// Helper function to create mock nodes
func createNodeResourceInfo(name, instanceType string, cpu, memory int64) planner.NodeResourceInfo {
	return planner.NodeResourceInfo{
		Name:         name,
		InstanceType: instanceType,
		Allocatable: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: memory,
		},
	}
}

// Helper function to create mock pods with cpu and memory requests
func createPodResourceInfo(name string, cpu, memory int64) planner.PodResourceInfo {
	return planner.PodResourceInfo{
		UID: "pod-12345",
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		AggregatedRequests: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: memory,
		},
	}
}

// Helper weights function for testing
func testWeightsFunc(_ string) (map[corev1.ResourceName]float64, error) {
	return map[corev1.ResourceName]float64{corev1.ResourceCPU: 5, corev1.ResourceMemory: 1}, nil
}

type testInfoAccess struct {
	err error
}

// Helper function to create stub instance pricing access that returns an error
func (m *testInfoAccess) GetInfo(_, _ string) (info planner.InstancePriceInfo, err error) {
	return planner.InstancePriceInfo{}, m.err
}
