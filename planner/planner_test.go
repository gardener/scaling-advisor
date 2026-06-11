// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/planner/testutil"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/samples"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	storagevolume "k8s.io/component-helpers/storage/volume"
)

func TestOnePoolUnitScaleOut(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleOutWithBoundPVC(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
		VolGenInput: samples.VolGenInput{
			PVCNames:   []string{"stem"},
			ClaimPhase: corev1.ClaimBound,
			GeneratePV: true,
		},
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleOutWithUnboundPVC_ExistingPV_ImmediateVolumeBinding(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
		VolGenInput: samples.VolGenInput{
			PVCNames:          []string{"stem"},
			ClaimPhase:        corev1.ClaimPending,
			VolumeBindingMode: storagev1.VolumeBindingImmediate,
			GeneratePV:        true,
		},
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleOutWithUnboundPVC_SimulatePV_ImmediateVolumeBinding(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
		VolGenInput: samples.VolGenInput{
			PVCNames:          []string{"stem"},
			ClaimPhase:        corev1.ClaimPending,
			VolumeBindingMode: storagev1.VolumeBindingImmediate,
		},
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleOutWithUnboundPVC_ExistingPV_WaitForFirstConsumer(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
		VolGenInput: samples.VolGenInput{
			PVCNames:          []string{"stem"},
			ClaimPhase:        corev1.ClaimPending,
			VolumeBindingMode: storagev1.VolumeBindingWaitForFirstConsumer,
			GeneratePV:        true,
		},
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleOutWithUnboundPVC_SimulatePV_WaitForFirstConsumer(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
		VolGenInput: samples.VolGenInput{
			PVCNames:          []string{"stem"},
			ClaimPhase:        corev1.ClaimPending,
			VolumeBindingMode: storagev1.VolumeBindingWaitForFirstConsumer,
		},
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestReusePlannerAcrossRequests(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: 1,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         1,
			},
		},
	}
	testData.Request.ID = t.Name() + "-A"
	if !testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan) {
		return
	}

	testData.Request.ID = t.Name() + "-B"
	if !testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan) {
		return
	}
}

func TestOnePoolFullFitPodScaleout(t *testing.T) {
	amount := 1
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: amount,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         int32(amount),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestOnePoolHalfFitPodScaleout tests scale out of one pool using HalfBerry pods that half-fit into pool A's NodeTemplate.
func TestOnePoolHalfFitPodScaleout(t *testing.T) {
	amount := 2
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetHalfBerry: amount,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         int32(amount / 2),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestOnePoolHalfAndFullFitPodScaleout tests scale out of one pool using both HalfBerry and Berry pods that half-fit
// and full-fit into pool A's NodeTemplate.
func TestOnePoolHalfAndFullFitPodScaleout(t *testing.T) {
	amount := 2
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetHalfBerry: amount,
			samples.ResourcePresetBerry:     amount,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         int32(math.Round(float64(amount) * 1.5)),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestTwoPoolFullFitPodScaleOut tests the scale-out scenarios for basic variant with 2 pools, where there is only one node template for each pool
// and where any unscheduled pod nearly fully fits into the node template.
func TestTwoPoolFullFitPodScaleOut(t *testing.T) {
	amount := 1
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset: samples.PoolPreset2P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{
			samples.ResourcePresetBerry: amount,
			samples.ResourcePresetGrape: amount,
		},
		Factories: NewFactories(),
	})
	if !ok {
		return
	}
	poolAPlacement, poolBPlacement := testData.NodePlacements[0], testData.NodePlacements[1]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: poolAPlacement,
				Delta:         int32(amount),
			},
			{
				NodePlacement: poolBPlacement,
				Delta:         int32(amount),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// ---- Scale-In tests ---------------------------------------------------------

func TestOnePoolScaleIn_SingleUnderutilizedNode(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	// Two nodes: node-a has low utilization, node-b has high utilization.
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}
	// node-a: tiny pod (~5% CPU, ~2% memory) → well below 50% threshold
	// node-b: large pod (~90% CPU, ~90% memory) → above threshold, won't be candidate
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		testutil.MakeScheduledPodInfo("pod-small", "node-a", "100m", "128Mi"),
		testutil.MakeScheduledPodInfo("pod-big", "node-b", "1700m", "6Gi"),
	}

	// Memento: node-a was seen as underutilized 10 minutes ago (exceeds the 5-min UnderutilizedDuration)
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	wantPlan := &sacorev1alpha1.ScaleInPlan{
		Items: []sacorev1alpha1.ScaleInItem{
			{
				NodePlacement: testData.NodePlacements[0],
				NodeName:      "node-a",
			},
		},
	}
	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, wantPlan)
}

func TestOnePoolScaleIn_NoScaleIn_AllHighUtilization(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	// Both nodes highly utilized → no scale-in candidates
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		testutil.MakeScheduledPodInfo("pod-a", "node-a", "1700m", "6Gi"),
		testutil.MakeScheduledPodInfo("pod-b", "node-b", "1700m", "6Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{},
		},
	}

	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, nil)
}

func TestOnePoolScaleIn_NoScaleIn_UnderutilizedDurationNotMet(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	// node-a is underutilized but first seen only 1 minute ago (below the 5-min threshold)
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		testutil.MakeScheduledPodInfo("pod-small", "node-a", "100m", "128Mi"),
		testutil.MakeScheduledPodInfo("pod-big", "node-b", "1700m", "6Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-1 * time.Minute),
			},
		},
	}

	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, nil)
}

func TestOnePoolScaleIn_NoScaleIn_FirstTimeSeen(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	// node-a is underutilized but never seen before (no memento entry)
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		testutil.MakeScheduledPodInfo("pod-small", "node-a", "100m", "128Mi"),
		testutil.MakeScheduledPodInfo("pod-big", "node-b", "1700m", "6Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{},
		},
	}

	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, nil)
}

// TestOnePoolScaleIn_PodWithBoundPVC_PVCUnbound verifies that when the scale-in candidate node
// holds a pod referencing a real bound PVC, the simulation unbinds the PVC (clears AnnSelectedNode)
// so the pod can reschedule to the remaining node, and the scale-in plan is produced.
func TestOnePoolScaleIn_PodWithBoundPVC_PVCUnbound(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	pv := testutil.MakePV("pv-stem", &corev1.ObjectReference{
		Namespace: "default",
		Name:      "pvc-stem",
	}, false)
	pv.Spec.StorageClassName = "standard"
	pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      corev1.LabelTopologyZone,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"eu-west-1c"},
				}},
			}},
		},
	}
	pvc := testutil.MakeBoundPVC("pvc-stem", "default", "pv-stem", map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	pvc.Spec.StorageClassName = &pv.Spec.StorageClassName
	pvc.Spec.Resources = corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
	}

	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}
	testData.Request.Snapshot.PVs = []plannerapi.PVInfo{volutil.AsPVInfo(*pv)}
	testData.Request.Snapshot.PVCs = []plannerapi.PVCInfo{volutil.AsPVCInfo(*pvc)}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		{
			ObjectMeta:         metav1.ObjectMeta{Name: "pod-with-pvc", Namespace: "default"},
			NodeName:           "node-a",
			SchedulerName:      "bin-packing-scheduler",
			AggregatedRequests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Volumes: []corev1.Volume{{
				Name:         "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-stem"}},
			}},
		},
		testutil.MakeScheduledPodInfo("pod-filler", "node-b", "1100m", "4Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	wantPlan := &sacorev1alpha1.ScaleInPlan{
		Items: []sacorev1alpha1.ScaleInItem{
			{
				NodePlacement: testData.NodePlacements[0],
				NodeName:      "node-a",
			},
		},
	}
	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, wantPlan)
}

// TestOnePoolScaleIn_PodWithSimulatedWFFCPVC_AllowedTopologyPreventsReschedule verifies that when
// the scale-in candidate node holds a pod referencing a simulated WFFC-bound PV and the
// StorageClass has AllowedTopologies restricted to node-a's zone, UnbindPodVolumes deletes the
// simulated PV and resets the PVC to Pending. The scheduler's VolumeBinding plugin then enforces
// AllowedTopologies at the Reserve phase, correctly rejecting node-b in a different zone, so the
// pod cannot be rescheduled and the scale-in plan is nil.
func TestOnePoolScaleIn_PodWithSimulatedWFFCPVC_AllowedTopologyPreventsReschedule(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		PoolZones:                           [][]string{{"eu-west-1a", "eu-west-1b"}},
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	scName := "standard"
	sc := storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: scName},
		Provisioner:       "ebs.csi.aws.com",
		VolumeBindingMode: func() *storagev1.VolumeBindingMode { m := storagev1.VolumeBindingWaitForFirstConsumer; return &m }(),
		AllowedTopologies: []corev1.TopologySelectorTerm{{
			MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{{
				Key:    corev1.LabelTopologyZone,
				Values: []string{"eu-west-1a"},
			}},
		}},
	}

	pv := testutil.MakePV("simVol-default-pvc-stem", &corev1.ObjectReference{
		Namespace: "default",
		Name:      "pvc-stem",
	}, true)
	pv.Spec.StorageClassName = scName
	pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      corev1.LabelTopologyZone,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"eu-west-1a"},
				}},
			}},
		},
	}

	pvc := testutil.MakeBoundPVC("pvc-stem", "default", pv.Name, map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	pvc.Spec.StorageClassName = &scName

	testData.Request.Snapshot.StorageClasses = []storagev1.StorageClass{sc}
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a", testutil.NodeInfoOpts{Zone: "eu-west-1a"}),
		testutil.MakeNodeInfo("node-b", testutil.NodeInfoOpts{Zone: "eu-west-1b"}),
	}
	testData.Request.Snapshot.PVs = []plannerapi.PVInfo{volutil.AsPVInfo(*pv)}
	testData.Request.Snapshot.PVCs = []plannerapi.PVCInfo{volutil.AsPVCInfo(*pvc)}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		{
			ObjectMeta:         metav1.ObjectMeta{Name: "pod-with-pvc", Namespace: "default"},
			NodeName:           "node-a",
			SchedulerName:      "bin-packing-scheduler",
			AggregatedRequests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Volumes: []corev1.Volume{{
				Name:         "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-stem"}},
			}},
		},
		testutil.MakeScheduledPodInfo("pod-filler", "node-b", "1100m", "4Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	// AllowedTopologies restricts volumes to eu-west-1a; node-b is in eu-west-1b so the pod
	// cannot be rescheduled — scale-in must be blocked.
	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, nil)
}

// TestOnePoolScaleIn_PodWithSimulatedWFFCPVC_AllowedTopologyMatchesDestination verifies that when
// the StorageClass AllowedTopologies includes node-b's zone (eu-west-1b), the scheduler selects
// node-b at Reserve, doWork provisions a fresh simulated PV in eu-west-1b, and the scale-in plan
// is produced.
func TestOnePoolScaleIn_PodWithSimulatedWFFCPVC_AllowedTopologyMatchesDestination(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		PoolZones:                           [][]string{{"eu-west-1a", "eu-west-1b"}},
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	scName := "standard"
	sc := storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: scName},
		Provisioner:       "ebs.csi.aws.com",
		VolumeBindingMode: func() *storagev1.VolumeBindingMode { m := storagev1.VolumeBindingWaitForFirstConsumer; return &m }(),
		AllowedTopologies: []corev1.TopologySelectorTerm{{
			MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{{
				Key:    corev1.LabelTopologyZone,
				Values: []string{"eu-west-1b"},
			}},
		}},
	}

	pv := testutil.MakePV("simVol-default-pvc-stem", &corev1.ObjectReference{
		Namespace: "default",
		Name:      "pvc-stem",
	}, true)
	pv.Spec.StorageClassName = scName
	pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      corev1.LabelTopologyZone,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"eu-west-1a"},
				}},
			}},
		},
	}

	pvc := testutil.MakeBoundPVC("pvc-stem", "default", pv.Name, map[string]string{
		storagevolume.AnnSelectedNode: "node-a",
	})
	pvc.Spec.StorageClassName = &scName

	testData.Request.Snapshot.StorageClasses = []storagev1.StorageClass{sc}
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a", testutil.NodeInfoOpts{Zone: "eu-west-1a"}),
		testutil.MakeNodeInfo("node-b", testutil.NodeInfoOpts{Zone: "eu-west-1b"}),
	}
	testData.Request.Snapshot.PVs = []plannerapi.PVInfo{volutil.AsPVInfo(*pv)}
	testData.Request.Snapshot.PVCs = []plannerapi.PVCInfo{volutil.AsPVCInfo(*pvc)}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{
		{
			ObjectMeta:         metav1.ObjectMeta{Name: "pod-with-pvc", Namespace: "default"},
			NodeName:           "node-a",
			SchedulerName:      "bin-packing-scheduler",
			AggregatedRequests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Volumes: []corev1.Volume{{
				Name:         "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-stem"}},
			}},
		},
		testutil.MakeScheduledPodInfo("pod-filler", "node-b", "1100m", "4Gi"),
	}
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	// AllowedTopologies allows eu-west-1b where node-b lives; doWork provisions a fresh
	// simulated PV there and the pod reschedules successfully.
	wantPlan := &sacorev1alpha1.ScaleInPlan{
		Items: []sacorev1alpha1.ScaleInItem{{
			NodePlacement: testData.NodePlacements[0],
			NodeName:      "node-a",
		}},
	}
	testutil.ObtainAndAssertScaleInPlan(t, planner, &testData, wantPlan)
}

// TestOnePoolScaleIn_RejectedDueToPreemption_ControllerOwnedVictim verifies that scale-in is
// rejected when removing the candidate node forces a controller-owned pod on the remaining node
// to be preempted, and the controller's replacement pod cannot be rescheduled anywhere.
//
// Setup:
//   - node-a (scale-in candidate, low utilization): hosts pod-high-prio (900m / 2Gi, priority 1000).
//   - node-b (high utilization): hosts pod-low-prio (1500m / 4Gi, priority 0) owned by a
//     ReplicaSet. Free CPU on node-b is ~420m, insufficient for pod-high-prio's 900m request.
//
// Flow: removing node-a unbinds pod-high-prio. The kube-scheduler can only place it on node-b
// by preempting pod-low-prio, which the scheduler then deletes. Because pod-low-prio has a
// controller owner, the simulation re-creates it (modeling the ReplicaSet controller). The
// re-created pod cannot fit anywhere — node-a is gone and node-b is now occupied by
// pod-high-prio — so its rescheduling fails. IsSimulationSuccess returns false and the planner
// emits no scale-in plan.
func TestOnePoolScaleIn_RejectedDueToPreemption_ControllerOwnedVictim(t *testing.T) {
	planr, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}

	controllerTrue := true
	podHighPrio, podLowPrio := testutil.PreemptionScenarioPods([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "pod-low-prio-rs",
		UID:        "pod-low-prio-rs-uid",
		Controller: &controllerTrue,
	}})
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{podHighPrio, podLowPrio}

	// Memento: node-a was first identified as underutilized 10 minutes ago, exceeding the 5-min
	// UnderutilizedDuration so it qualifies as a scale-in candidate.
	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	// Drive the planner directly so we can observe rejection regardless of whether the
	// simulation produces a clean nil plan or surfaces the preemption-induced failure as an
	// error: in either case the outcome is "scale-in for node-a is not approved".
	responseCh := planr.Plan(testData.RunContext, testData.Request)
	response := <-responseCh
	if response.Error != nil && errors.Is(response.Error, plannerapi.ErrNoScaleInPlan) {
		t.Logf("scale-in rejected via simulation error: %v", response.Error)
		return
	}
	if response.ScaleInPlan != nil && len(response.ScaleInPlan.Items) > 0 {
		t.Fatalf("expected no scale-in plan (controller-owned pod-low-prio's replacement cannot reschedule) but got %+v", response.ScaleInPlan)
	}
}

// TestOnePoolScaleIn_ApprovedAfterBarePodPreemption verifies the complementary case to
// TestOnePoolScaleIn_RejectedDueToPreemption_ControllerOwnedVictim: when the preempted pod is
// bare (no controller owner), no replacement is created in a real cluster, so the simulation
// has no follow-up scheduling to model. The displaced high-priority pod is successfully bound
// on the remaining node and the scale-in is approved.
//
// Setup is identical to the controller-owned case except pod-low-prio carries no
// OwnerReferences. After preemption the simulation does not attempt to recreate it; with only
// pod-high-prio left to place on node-b, scheduling succeeds and the simulation reports
// IsSimulationSuccess=true for node-a.
func TestOnePoolScaleIn_ApprovedAfterBarePodPreemption(t *testing.T) {
	planr, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{
		testutil.MakeNodeInfo("node-a"),
		testutil.MakeNodeInfo("node-b"),
	}

	podHighPrio, podLowPrio := testutil.PreemptionScenarioPods(nil)
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{podHighPrio, podLowPrio}

	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	// Bare pod-low-prio gets preempted and is gone forever (no controller to recreate it). The
	// scale-in succeeds: pod-high-prio is rescheduled onto node-b and node-a is approved.
	wantPlan := &sacorev1alpha1.ScaleInPlan{
		Items: []sacorev1alpha1.ScaleInItem{{
			NodePlacement: testData.NodePlacements[0],
			NodeName:      "node-a",
		}},
	}
	testutil.ObtainAndAssertScaleInPlan(t, planr, &testData, wantPlan)
}

// TestOnePoolScaleIn_ApprovedWithControllerOwnedVictimReschedulable is the positive complement
// to TestOnePoolScaleIn_RejectedDueToPreemption_ControllerOwnedVictim: the controller-owned
// preempted pod CAN be rescheduled, this time onto a third node that has spare capacity. The
// scale-in is therefore approved.
//
// Setup:
//   - node-a (scale-in candidate, low utilization) and node-b (high utilization) both carry the
//     label "tier=primary". node-a hosts pod-high-prio (900m / 2Gi, priority 1000) which has
//     NodeSelector{tier=primary}, restricting it to node-a or node-b. node-b hosts pod-low-prio
//     (1500m / 4Gi, priority 0) owned by a ReplicaSet, with no NodeSelector.
//   - node-c is empty, has no "tier" label, and is annotated with ScaleInDisabled so it is not
//     considered as a scale-in candidate. It is the only node where pod-low-prio's recreated
//     replica can land — pod-high-prio cannot go there because of its NodeSelector.
//
// Flow: removing node-a unbinds pod-high-prio. Its NodeSelector forces it onto node-b (node-c
// is excluded), but node-b doesn't have room alongside pod-low-prio, so the scheduler preempts
// pod-low-prio. The simulation re-creates pod-low-prio (modeling its owning ReplicaSet); the
// kube-scheduler schedules the replacement onto node-c which has plenty of capacity. All
// displaced pods land successfully → IsSimulationSuccess=true → scale-in for node-a is approved.
func TestOnePoolScaleIn_ApprovedWithControllerOwnedVictimReschedulable(t *testing.T) {
	planr, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolPreset:                          samples.PoolPreset1P,
		NumUnscheduledPodsPerResourcePreset: map[samples.ResourcePreset]int{},
		Factories:                           NewFactories(),
	})
	if !ok {
		return
	}

	// nodes a and b carry the "tier=primary" label that pod-high-prio's NodeSelector requires;
	// node-c does not, so pod-high-prio cannot land there. node-c is also annotated as
	// scale-in-disabled so the candidate selector ignores it (otherwise it would be eligible
	// alongside node-a and the simulator could pick it at random).
	nodeA := testutil.MakeNodeInfo("node-a")
	nodeA.Labels["tier"] = "primary"
	nodeB := testutil.MakeNodeInfo("node-b")
	nodeB.Labels["tier"] = "primary"
	nodeC := testutil.MakeNodeInfo("node-c")
	nodeC.Annotations = map[string]string{commonconstants.AnnotationScaleInDisabled: "true"}
	testData.Request.Snapshot.Nodes = []plannerapi.NodeInfo{nodeA, nodeB, nodeC}

	controllerTrue := true
	podHighPrio, podLowPrio := testutil.PreemptionScenarioPods([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "pod-low-prio-rs",
		UID:        "pod-low-prio-rs-uid",
		Controller: &controllerTrue,
	}})
	// Constrain pod-high-prio to nodes a/b so it must preempt rather than spill onto node-c.
	podHighPrio.NodeSelector = map[string]string{"tier": "primary"}
	testData.Request.Snapshot.Pods = []plannerapi.PodInfo{podHighPrio, podLowPrio}

	testData.Request.Memento = plannerapi.Memento{
		ScaleIn: plannerapi.ScaleInMemento{
			LastIdentifiedUnneededNodes: map[string]time.Time{
				"node-a": time.Now().Add(-10 * time.Minute),
			},
		},
	}

	// pod-low-prio's recreated replica reschedules onto node-c, so the scale-in for node-a is
	// approved.
	wantPlan := &sacorev1alpha1.ScaleInPlan{
		Items: []sacorev1alpha1.ScaleInItem{{
			NodePlacement: testData.NodePlacements[0],
			NodeName:      "node-a",
		}},
	}
	testutil.ObtainAndAssertScaleInPlan(t, planr, &testData, wantPlan)
}
