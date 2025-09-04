package scorer

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/gardener/scaling-advisor/service/pricing/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func CreateMockNode(name, instanceType string, cpu, memory int64) api.NodeResourceInfo {
	return api.NodeResourceInfo{
		Name:         name,
		InstanceType: instanceType,
		Allocatable: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: memory,
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
		AggregatedRequests: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: memory,
		},
	}
}

// Helper function to create mock weights for instance type
func NewMockWeightsFunc(instanceType string) (map[corev1.ResourceName]float64, error) {
	return map[corev1.ResourceName]float64{corev1.ResourceCPU: 5, corev1.ResourceMemory: 1}, nil
}
func TestLeastWasteScoringStrategy(t *testing.T) {
	access, err := testutil.LoadTestInstanceTypeInfoAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.LeastWasteNodeScoringStrategy, access, NewMockWeightsFunc)
	if err != nil {
		t.Fatal(err)
		return
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
				ID:               "testing",
				Placement:        api.NodePlacementInfo{},
				ScaledAssignment: &assignment,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				ID:                 "testing",
				Placement:          api.NodePlacementInfo{},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: api.NodeScoreArgs{
				ID:               "testing",
				Placement:        api.NodePlacementInfo{},
				ScaledAssignment: &assignment,
				OtherAssignments: []api.NodePodAssignment{{
					Node:          CreateMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []api.PodResourceInfo{CreateMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				ID:                 "testing",
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
	access, err := testutil.LoadTestInstanceTypeInfoAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	scorer, err := GetNodeScorer(commontypes.LeastCostNodeScoringStrategy, access, NewMockWeightsFunc)
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
				ID:               "testing",
				Placement:        api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
				ScaledAssignment: &assignment,
				OtherAssignments: nil,
				UnscheduledPods:  nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				ID:                 "testing",
				Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignment.Node,
			},
		},
		"pods scheduled on scaled node and existing node": {
			input: api.NodeScoreArgs{
				ID:               "testing",
				Placement:        api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
				ScaledAssignment: &assignment,
				OtherAssignments: []api.NodePodAssignment{{
					Node:          CreateMockNode("exNode1", "instance-b-1", 2, 4),
					ScheduledPods: []api.PodResourceInfo{CreateMockPod("simPodB", 1, 2)},
				}},
				UnscheduledPods: nil},
			expectedErr: nil,
			expectedScore: api.NodeScore{
				ID:                 "testing",
				Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
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
	access, err := testutil.LoadTestInstanceTypeInfoAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	selector, err := GetNodeScoreSelector(commontypes.LeastCostNodeScoringStrategy)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []api.NodeScore
		expectedErr error
		expectedIn  []api.NodeScore
	}{
		"single node score": {
			input:       []api.NodeScore{{ID: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []api.NodeScore{{ID: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []api.NodeScore{},
			expectedErr: api.ErrNoWinningNodeScore,
			expectedIn:  []api.NodeScore{},
		},
		"different allocatables": {
			input: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 4, 8),
				}},
			expectedErr: nil,
			expectedIn: []api.NodeScore{{
				ID:                 "testing2",
				Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              1,
				ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 4, 8),
			}},
		},
		"identical allocatables": {
			input: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 2, 4),
				},
			},
			expectedErr: nil,
			expectedIn: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 2, 4),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := selector(tc.input, NewMockWeightsFunc, access)
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
	access, err := testutil.LoadTestInstanceTypeInfoAccess()
	if err != nil {
		t.Fatal(err)
		return
	}
	selector, err := GetNodeScoreSelector(commontypes.LeastWasteNodeScoringStrategy)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []api.NodeScore
		expectedErr error
		expectedIn  []api.NodeScore
	}{
		"single node score": {
			input:       []api.NodeScore{{ID: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
			expectedErr: nil,
			expectedIn:  []api.NodeScore{{ID: "testing", Placement: api.NodePlacementInfo{}, UnscheduledPods: nil, Value: 1, ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"no node score": {
			input:       []api.NodeScore{},
			expectedErr: api.ErrNoWinningNodeScore,
			expectedIn:  []api.NodeScore{},
		},
		"different prices": {
			input: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-2"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-a-2", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)}},
		},
		"identical prices": {
			input: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-c-1", 1, 2),
				},
			},
			expectedErr: nil,
			expectedIn: []api.NodeScore{
				{
					ID:                 "testing1",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-a-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode1", "instance-a-1", 2, 4)},
				{
					ID:                 "testing2",
					Placement:          api.NodePlacementInfo{Region: "s", InstanceType: "instance-c-1"},
					UnscheduledPods:    nil,
					Value:              1,
					ScaledNodeResource: CreateMockNode("simNode2", "instance-c-1", 1, 2),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := selector(tc.input, NewMockWeightsFunc, access)
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
