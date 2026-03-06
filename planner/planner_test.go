// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"math"
	"testing"

	"github.com/gardener/scaling-advisor/planner/testutil"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/samples"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
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
