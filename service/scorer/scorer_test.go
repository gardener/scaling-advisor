// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scorer

import (
	"errors"
	"reflect"
	"testing"

	prtestutil "github.com/gardener/scaling-advisor/service/pricing/testutil"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/service"
	testutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Helper function to create mock nodes with allocatable
func createMockNode(name, instanceType string, cpu, memory int64) service.NodeResourceInfo {
	return service.NodeResourceInfo{
		Name:         name,
		InstanceType: instanceType,
		Allocatable: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: memory,
		},
	}
}

// Helper function to create mock pods with cpu and memory requests
func createMockPod(name string, cpu, memory int64) service.PodResourceInfo {
	return service.PodResourceInfo{
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

// Helper function to create mock weights for instance type
func newMockWeightsFunc(_ string) (map[corev1.ResourceName]float64, error) {
	return map[corev1.ResourceName]float64{corev1.ResourceCPU: 5, corev1.ResourceMemory: 1}, nil
}

func TestLeastWasteScoringStrategy(t *testing.T) {
	access, err := prtestutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.LeastWasteNodeScoringStrategy, access, newMockWeightsFunc)
	if err != nil {
		t.Fatal(err)
		return
	}
	assignment := service.NodePodAssignment{
		Node: createMockNode("simNode1", "instance-a-1", 2, 4),
		ScheduledPods: []service.PodResourceInfo{
			createMockPod("simPodA", 1, 2),
		},
	}
	//test case where weights are not defined for all resources
	podWithStorage := createMockPod("simStorage", 2, 4)
	podWithStorage.AggregatedRequests["Storage"] = 10
	assignmentWithStorage := service.NodePodAssignment{
		Node:          createMockNode("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []service.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         service.NodeScorerArgs
		expectedErr   error
		expectedScore service.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{},
				ScaledAssignment: &assignment,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{},
				ScaledAssignment: &assignment,
				OtherAssignments: []service.NodePodAssignment{{
					Node:          createMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []service.PodResourceInfo{createMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignment.Node,
			},
		},

		"weights undefined for resource type": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{},
				ScaledAssignment: &assignmentWithStorage,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignmentWithStorage.Node,
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
	access, err := prtestutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.LeastCostNodeScoringStrategy, access, newMockWeightsFunc)
	if err != nil {
		t.Fatal(err)
	}
	assignment := service.NodePodAssignment{
		Node: createMockNode("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []service.PodResourceInfo{
			createMockPod("simPodA", 1, 2),
		},
	}
	//test case where weights are not defined for all resources
	podWithStorage := createMockPod("simStorage", 1, 2)
	podWithStorage.AggregatedRequests["Storage"] = 10
	assignmentWithStorage := service.NodePodAssignment{
		Node:          createMockNode("simNode1", "instance-a-2", 2, 4),
		ScheduledPods: []service.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         service.NodeScorerArgs
		expectedErr   error
		expectedScore service.NodeScore
	}{
		"pod scheduled on scaled node only": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledAssignment: &assignment,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledAssignment: &assignment,
				OtherAssignments: []service.NodePodAssignment{{
					Node:          createMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []service.PodResourceInfo{createMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.Node,
			},
		},
		"weights undefined for resource type": {
			input: service.NodeScorerArgs{
				ID:               "testing",
				Placement:        sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				ScaledAssignment: &assignmentWithStorage,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: service.NodeScore{
				ID:                 "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignmentWithStorage.Node,
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
	access, err := prtestutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	selector, err := GetNodeScoreSelector(commontypes.LeastCostNodeScoringStrategy)
	simNodeWithStorage := createMockNode("simNode1", "instance-a-1", 2, 4)
	simNodeWithStorage.Allocatable["Storage"] = 10
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []service.NodeScore
		expectedErr error
		expectedIn  []service.NodeScore
	}{
		"single node score": {
			input:       []service.NodeScore{{ID: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []service.NodeScore{{ID: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []service.NodeScore{},
			expectedErr: service.ErrNoWinningNodeScore,
			expectedIn:  []service.NodeScore{},
		},
		"different allocatables": {
			input: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-a-2", 4, 8),
				}},
			expectedErr: nil,
			expectedIn: []service.NodeScore{{
				ID:                 "testing2",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              1,
				ScaledNodeResource: createMockNode("simNode2", "instance-a-2", 4, 8),
			}},
		},
		"identical allocatables": {
			input: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-a-2", 2, 4),
				},
			},
			expectedErr: nil,
			expectedIn: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-a-2", 2, 4),
				},
			},
		},
		"undefined weights for resource type": {
			input: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 4, 8)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: simNodeWithStorage,
				}},
			expectedErr: nil,
			expectedIn: []service.NodeScore{{
				ID:                 "testing1",
				Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
				UnscheduledPods:    nil,
				Value:              1,
				ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 4, 8),
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := selector(tc.input, newMockWeightsFunc, access)
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
	access, err := prtestutil.LoadTestInstancePricingAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	selector, err := GetNodeScoreSelector(commontypes.LeastWasteNodeScoringStrategy)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []service.NodeScore
		expectedErr error
		expectedIn  []service.NodeScore
	}{
		"single node score": {
			input:       []service.NodeScore{{ID: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []service.NodeScore{{ID: "testing", Placement: sacorev1alpha1.NodePlacement{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []service.NodeScore{},
			expectedErr: service.ErrNoWinningNodeScore,
			expectedIn:  []service.NodeScore{},
		},
		"different prices": {
			input: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-a-2", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"identical prices": {
			input: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-c-1", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []service.NodeScore{
				{
					ID:                 "testing1",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          sacorev1alpha1.NodePlacement{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: createMockNode("simNode2", "instance-c-1", 1, 2),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := selector(tc.input, newMockWeightsFunc, access)
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

func TestGetNodeScoreSelector(t *testing.T) {
	tests := map[string]struct {
		input                commontypes.NodeScoringStrategy
		expectedFunctionName string
		expectedError        error
	}{
		"least-cost strategy": {
			input:                commontypes.LeastCostNodeScoringStrategy,
			expectedFunctionName: testutil.GetFunctionName(t, SelectMaxAllocatable),
			expectedError:        nil,
		},
		"least-waste strategy": {
			input:                commontypes.LeastWasteNodeScoringStrategy,
			expectedFunctionName: testutil.GetFunctionName(t, SelectMinPrice),
			expectedError:        nil,
		},
		"invalid strategy": {
			input:                "invalid",
			expectedFunctionName: "",
			expectedError:        service.ErrUnsupportedNodeScoringStrategy,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := GetNodeScoreSelector(tc.input)
			gotFunctionName := testutil.GetFunctionName(t, got)
			if tc.expectedError == nil {
				if err != nil {
					t.Fatalf("Expected error to be nil but got %v", err)
				}
			} else if tc.expectedError != nil {
				if err != nil && !errors.Is(err, tc.expectedError) {
					t.Fatalf("Expected error to wrap %v but got %v", tc.expectedError, err)
				} else if err == nil {
					t.Fatalf("Expected error to be %v but got nil", tc.expectedError)
				}
			}
			if tc.expectedFunctionName != "" {
				if got == nil {
					t.Fatalf("Expected node score selector to be %s but got nil", gotFunctionName)
				} else {
					gotType := reflect.TypeOf(got).String()
					if gotFunctionName != tc.expectedFunctionName {
						t.Fatalf("Expected node score selector %s but got %s", tc.expectedFunctionName, gotType)
					}
				}
			}
		})
	}
}

func TestGetNodeScorer(t *testing.T) {
	tests := map[string]struct {
		input         commontypes.NodeScoringStrategy
		expectedType  string
		expectedError error
	}{
		"least-cost strategy": {
			input:         commontypes.LeastCostNodeScoringStrategy,
			expectedType:  "*scorer.LeastCost",
			expectedError: nil,
		},
		"least-waste strategy": {
			input:         commontypes.LeastWasteNodeScoringStrategy,
			expectedType:  "*scorer.LeastWaste",
			expectedError: nil,
		},
		"invalid strategy": {
			input:         "invalid",
			expectedType:  "",
			expectedError: service.ErrUnsupportedNodeScoringStrategy,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := GetNodeScorer(tc.input, nil, nil)
			if tc.expectedError == nil {
				if err != nil {
					t.Fatalf("Expected error to be nil but got %v", err)
				}
			} else if tc.expectedError != nil {
				if err != nil && !errors.Is(err, tc.expectedError) {
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
