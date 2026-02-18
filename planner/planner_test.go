// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/planner/testutil"
	"github.com/gardener/scaling-advisor/samples"
	"math"
	"testing"
)

func TestBasicOnePoolUnitScaleOut(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
		PlannerFactory: New,
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestBasicOnePoolScaleOutWithVolumeClaim(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
		PlannerFactory: New,
		PVCNames:       []string{"stem"},
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         1,
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

func TestReusePlannerAcrossRequests(t *testing.T) {
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
		PlannerFactory: New,
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
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

func TestBasicOnePoolFullFitPodScaleout(t *testing.T) {
	amount := 2
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: amount,
		},
		PlannerFactory: New,
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         int32(amount),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestBasicOnePoolHalfFitPodScaleout tests scale out of one pool using HalfBerry pods that half-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfFitPodScaleout(t *testing.T) {
	amount := 4
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryHalfBerry: amount,
		},
		PlannerFactory: New,
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         int32(amount / 2),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestBasicOnePoolHalfAndFullFitPodScaleout tests scale out of one pool using both HalfBerry and Berry pods that half-fit
// and full-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfAndFullFitPodScaleout(t *testing.T) {
	amount := 4
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryHalfBerry: amount,
			samples.ResourceCategoryBerry:     amount,
		},
		PlannerFactory: New,
	})
	if !ok {
		return
	}
	pPlacement := testData.NodePlacements[0]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         int32(math.Round(float64(amount) * 1.5)),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}

// TestBasicTwoPoolFullFitPodScaleOut tests the scale-out scenarios for basic variant with 2 pools, where there is only one node template for each pool
// and where any unscheduled pod nearly fully fits into the node template.
func TestBasicTwoPoolFullFitPodScaleOut(t *testing.T) {
	amount := 3
	planner, testData, ok := testutil.CreateTestPlannerAndTestData(t, testutil.Args{
		PoolCategory: samples.PoolCategoryBasicTwo,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: amount,
			samples.ResourceCategoryGrape: amount,
		},
		AdviceGenerationMode: commontypes.ScalingAdviceGenerationModeAllAtOnce,
		PlannerFactory:       New,
	})
	if !ok {
		return
	}
	pPlacement, qPlacement := testData.NodePlacements[0], testData.NodePlacements[1]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement: pPlacement,
				Delta:         int32(amount),
			},
			{
				NodePlacement: qPlacement,
				Delta:         int32(amount),
			},
		},
	}
	testutil.ObtainAndAssertScaleOutPlan(t, planner, &testData, wantPlan)
}
