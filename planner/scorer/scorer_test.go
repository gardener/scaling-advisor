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
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Instance types from fake-instance_price_infos.json (region "s"):
//
//	instance-a-1: 2 vCPU / 4 mem, price 1.0
//	instance-a-2: 4 vCPU / 8 mem, price 2.0
//	instance-b-1: 8 vCPU / 16 mem, price 4.0
//	instance-b-2: 16 vCPU / 32 mem, price 8.0
//	instance-c-1: 2 vCPU / 4 mem,  price 1.0 (same price as instance-a-1)
const region = "s"

func TestLeastWasteScoringStrategy(t *testing.T) {
	access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
	if err != nil {
		t.Fatal(err)
		return
	}
	// node: instance-a-1 (2 CPU / 4 mem), pod A: 1 CPU / 2 mem
	// wastage = (2-1) CPU + (4-2) mem = 1 CPU + 2 mem → NRU = 5*1 + 1*2 = 7 → score = int(7*100) = 700
	assignment := plannerapi.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4"),
		ScheduledPods: []plannerapi.PodResourceInfo{
			createPodResourceInfo("simPodA", "1", "2"),
		},
	}
	// node: instance-a-2 (4 CPU / 8 mem), pod: 4 CPU / 8 mem + 10 storage (no weight)
	// wastage = (4-4) CPU + (8-8) mem = 0 → score = 0
	podWithStorage := createPodResourceInfo("simStorage", "4", "8")
	podWithStorage.AggregatedRequests[corev1.ResourceStorage] = resource.MustParse("10")
	assignmentWithStorage := plannerapi.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8"),
		ScheduledPods: []plannerapi.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         plannerapi.NodeScorerArgs
		access        pricingapi.InstancePricingAccess
		weigher       plannerapi.ResourceWeigher
		expectedErr   error
		expectedScore plannerapi.NodeScore
	}{
		// wastage = (2-1) CPU + (4-2) mem = (1, 2) → NRU = 5*1 + 1*2 = 7 → score = int(700) = 700
		"pod scheduled on scaled node only": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		// wastage = allocatable - (pod A on scaled node + pod B on existing node)
		// = (2,4) - (1,2) - (1,2) = (0,0) → score = 0
		"pods scheduled on scaled node and existing node": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: []plannerapi.NodePodAssignment{{
					NodeResources: createNodeResourceInfo("exNode1", "instance-b-1", "8", "16"),
					ScheduledPods: []plannerapi.PodResourceInfo{createPodResourceInfo("simPodB", "1", "2")},
				}},
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		// storage has no weight → NRU for storage = 0; wastage = (4-4) CPU + (8-8) mem = 0 → score = 0
		"weights undefined for resource type": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignmentWithStorage,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              0,
				ScaledNodeResource: assignmentWithStorage.NodeResources,
			},
		},
		"weights function returns an error": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:        access,
			weigher:       &testWeigher{inError: true},
			expectedErr:   plannerapi.ErrComputeNodeScore,
			expectedScore: plannerapi.NodeScore{},
		},
		"pricingAccess.GetInfo() function returns an error": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:        &testInfoAccess{err: errors.New("testing error")},
			weigher:       &testWeigher{inError: true},
			expectedErr:   plannerapi.ErrComputeNodeScore,
			expectedScore: plannerapi.NodeScore{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastWaste, tc.access, tc.weigher)
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
	// node: instance-a-2 (4 CPU / 8 mem, price 2.0), pod A: 1 CPU / 2 mem
	// NRU(pod A) = 5*1 + 1*2 = 7 → score = round(7*100/2.0) = 350
	assignment := plannerapi.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-2", "4", "8"),
		ScheduledPods: []plannerapi.PodResourceInfo{
			createPodResourceInfo("simPodA", "1", "2"),
		},
	}
	// pod: 1 CPU / 2 mem + 10 storage (storage has no weight)
	// NRU = 5*1 + 1*2 = 7 (storage ignored) → score = round(700/2.0) = 350
	podWithStorage := createPodResourceInfo("simStorage", "1", "2")
	podWithStorage.AggregatedRequests[corev1.ResourceStorage] = resource.MustParse("10")
	assignmentWithStorage := plannerapi.NodePodAssignment{
		NodeResources: createNodeResourceInfo("simNode1", "instance-a-2", "4", "8"),
		ScheduledPods: []plannerapi.PodResourceInfo{podWithStorage},
	}
	tests := map[string]struct {
		input         plannerapi.NodeScorerArgs
		access        pricingapi.InstancePricingAccess
		weigher       plannerapi.ResourceWeigher
		expectedErr   error
		expectedScore plannerapi.NodeScore
	}{
		// pod A on scaled node only: NRU = 5*1 + 1*2 = 7 → score = round(700/2.0) = 350
		"pod scheduled on scaled node only": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		// pod A on scaled node + pod B on existing node: NRU = 5*(1+1) + 1*(2+2) = 14 → score = round(1400/2.0) = 700
		"pods scheduled on scaled node and existing node": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: []plannerapi.NodePodAssignment{{
					NodeResources: createNodeResourceInfo("exNode1", "instance-b-1", "8", "16"),
					ScheduledPods: []plannerapi.PodResourceInfo{createPodResourceInfo("simPodB", "1", "2")},
				}},
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              700,
				ScaledNodeResource: assignment.NodeResources,
			},
		},
		// storage has no weight → NRU = 5*1 + 1*2 = 7 (storage ignored) → score = round(700/2.0) = 350
		"weights undefined for resource type": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignmentWithStorage,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:      access,
			weigher:     &testWeigher{},
			expectedErr: nil,
			expectedScore: plannerapi.NodeScore{
				Name:               "testing",
				Placement:          sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				UnscheduledPods:    nil,
				Value:              350,
				ScaledNodeResource: assignmentWithStorage.NodeResources,
			},
		},
		"weights function returns an error": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:        access,
			weigher:       &testWeigher{inError: true},
			expectedErr:   plannerapi.ErrComputeNodeScore,
			expectedScore: plannerapi.NodeScore{},
		},
		"pricingAccess.GetInfo() function returns an error": {
			input: plannerapi.NodeScorerArgs{
				ID:                      "testing",
				ScaledNodePlacement:     sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"},
				ScaledNodePodAssignment: &assignment,
				OtherNodePodAssignments: nil,
				LeftOverUnscheduledPods: nil},
			access:        &testInfoAccess{err: errors.New("testing error")},
			weigher:       &testWeigher{inError: true},
			expectedErr:   plannerapi.ErrComputeNodeScore,
			expectedScore: plannerapi.NodeScore{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, tc.access, tc.weigher)
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
	scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, access, &testWeigher{})
	if err != nil {
		t.Fatal(err)
	}
	// instance-a-2 node with extra storage resource that has no weight defined
	simNodeWithStorage := createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")
	simNodeWithStorage.Allocatable[corev1.ResourceStorage] = resource.MustParse("10")
	tests := map[string]struct {
		input       []plannerapi.NodeScore
		expectedErr error
		expectedIn  []plannerapi.NodeScore
	}{
		"single node score": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
		},
		"no node score": {
			input:       []plannerapi.NodeScore{},
			expectedErr: plannerapi.ErrNoWinningNodeScore,
			expectedIn:  []plannerapi.NodeScore{},
		},
		// both value=1; instance-a-2 (4 CPU / 8 mem) > instance-a-1 (2 CPU / 4 mem) → instance-a-2 wins
		"different allocatables": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")},
			},
		},
		// both value=1 and identical allocatable (2 CPU / 4 mem) → either may be returned
		"identical allocatables": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", "2", "4")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", "2", "4")},
			},
		},
		// both value=1; instance-c-2: 4 CPU / 8 mem; instance-a-2: 4 CPU / 8 mem + storage (storage ignored)
		// NRU(instance-c-2) = 5*4 + 1*8 = 28; NRU(instance-a-2) = 5*4 + 1*8 = 28 (storage ignored)
		// tied → either may be returned
		"undefined weights for resource type": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-2"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-c-2", "4", "8")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 1, ScaledNodeResource: simNodeWithStorage},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-2"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-c-2", "4", "8")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 1, ScaledNodeResource: simNodeWithStorage},
			},
		},
		// sim1: value=10, instance-a-2 (4 CPU / 8 mem); sim2: value=20, instance-a-1 (2 CPU / 4 mem)
		// highest value wins regardless of allocatable → sim2 wins
		"different values: highest value wins regardless of allocatable": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-2", "4", "8")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 20, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-1", "2", "4")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 20, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-1", "2", "4")},
			},
		},
		// sim1: value=10, instance-a-1 (2 CPU / 4 mem); sim2: value=10, instance-a-2 (4 CPU / 8 mem); sim3: value=5, instance-b-1 (8 CPU / 16 mem)
		// sim1 and sim2 tie on value; sim2 has larger allocatable → sim2 wins; sim3 excluded despite largest allocatable
		"tied value: larger allocatable wins": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")},
				{Name: "sim3", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-b-1"}, Value: 5, ScaledNodeResource: createNodeResourceInfo("simNode3", "instance-b-1", "8", "16")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := scorer.Select(logr.Discard(), tc.input)
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
	scorer, err := GetNodeScorer(commontypes.NodeScoringStrategyLeastWaste, access, &testWeigher{})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		input       []plannerapi.NodeScore
		expectedErr error
		expectedIn  []plannerapi.NodeScore
	}{
		"single node score": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
		},
		"no node score": {
			input:       []plannerapi.NodeScore{},
			expectedErr: plannerapi.ErrNoWinningNodeScore,
			expectedIn:  []plannerapi.NodeScore{},
		},
		// both value=1; instance-a-1 (price 1.0) < instance-a-2 (price 2.0) → instance-a-1 wins
		"different prices": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-2", "4", "8")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
		},
		// both value=1; instance-a-1 and instance-c-1 both have price 1.0 → either may be returned
		"identical prices": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", "2", "4")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-c-1"}, Value: 1, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-c-1", "2", "4")},
			},
		},
		// sim1: value=10, instance-a-1 (price 1.0); sim2: value=20, instance-b-2 (price 8.0)
		// lowest value wins regardless of price → sim1 wins
		"different values: lowest value wins regardless of price": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-b-2"}, Value: 20, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-b-2", "16", "32")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-a-1", "2", "4")},
			},
		},
		// sim1: value=5, instance-b-2 (price 8.0); sim2: value=5, instance-a-1 (price 1.0); sim3: value=10, instance-a-2 (price 2.0)
		// sim1 and sim2 tie on lowest value; sim2 is cheaper → sim2 wins; sim3 excluded despite lower price
		"tied value: cheapest price wins": {
			input: []plannerapi.NodeScore{
				{Name: "sim1", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-b-2"}, Value: 5, ScaledNodeResource: createNodeResourceInfo("simNode1", "instance-b-2", "16", "32")},
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 5, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-1", "2", "4")},
				{Name: "sim3", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-2"}, Value: 10, ScaledNodeResource: createNodeResourceInfo("simNode3", "instance-a-2", "4", "8")},
			},
			expectedErr: nil,
			expectedIn: []plannerapi.NodeScore{
				{Name: "sim2", Placement: sacorev1alpha1.NodePlacement{Region: region, InstanceType: "instance-a-1"}, Value: 5, ScaledNodeResource: createNodeResourceInfo("simNode2", "instance-a-1", "2", "4")},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			winningNodeScore, err := scorer.Select(logr.Discard(), tc.input)
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
			expectedError: plannerapi.ErrCreateNodeScorer,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			access, err := pricingtestutil.GetInstancePricingAccessWithFakeData()
			if err != nil {
				t.Fatalf("GetInstancePricingAccessWithFakeData failed with error: %v", err)
			}
			got, err := GetNodeScorer(tc.input, access, &testWeigher{})
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
func createNodeResourceInfo(name, instanceType string, cpu, memory string) plannerapi.NodeResourceInfo {
	return plannerapi.NodeResourceInfo{
		Name:         name,
		InstanceType: instanceType,
		Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
	}
}

// Helper function to create mock pods with cpu and memory requests
func createPodResourceInfo(name string, cpu, memory string) plannerapi.PodResourceInfo {
	return plannerapi.PodResourceInfo{
		NamespacedName: commontypes.NamespacedName{Namespace: metav1.NamespaceDefault, Name: name},
		AggregatedRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
	}
}

var _ plannerapi.ResourceWeigher = (*testWeigher)(nil)

type testWeigher struct {
	inError bool
}

func (t *testWeigher) GetWeights(_ string) (map[corev1.ResourceName]float64, error) {
	if t.inError {
		return nil, errors.New("testing error")
	}
	return map[corev1.ResourceName]float64{corev1.ResourceCPU: 5, corev1.ResourceMemory: 1}, nil
}

type testInfoAccess struct {
	err error
}

// Helper function to create stub instance pricing access that returns an error
func (m *testInfoAccess) GetInfo(_, _ string) (info pricingapi.InstancePriceInfo, err error) {
	return pricingapi.InstancePriceInfo{}, m.err
}
