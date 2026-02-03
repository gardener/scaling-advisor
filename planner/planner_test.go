// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/planner/scheduler"
	"github.com/gardener/scaling-advisor/planner/weights"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

const defaultTestVerbosity = 0

const defaultPlannerTimeout = 30 * time.Second

// TestArgs represents the common test args for the scale-out unit-tests of the ScalingPlanner
type TestArgs struct {
	NumUnscheduledPerResourceCategory map[samples.ResourceCategory]int
	PoolCategory                      samples.PoolCategory
	SimulatorStrategy                 commontypes.SimulatorStrategy
	NodeScoringStrategy               commontypes.NodeScoringStrategy
	AdviceGenerationMode              commontypes.ScalingAdviceGenerationMode
	Timeout                           time.Duration
}

// TestData holds all the common test data necessary for carrying out the scale-out unit-tests of the ScalingPlanner and asserting conditions
type TestData struct {
	Planner        plannerapi.ScalingPlanner
	RunContext     context.Context
	SnapshotPath   string
	NodePlacements []sacorev1alpha1.NodePlacement
	Request        plannerapi.Request
}

func TestBasicOnePoolUnitScaleOut(t *testing.T) {
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func TestReusePlannerAcrossRequests(t *testing.T) {
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
	testData.Request.ID = t.Name() + "-B"
	gotPlan = obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func TestBasicOnePoolFullFitPodScaleout(t *testing.T) {
	amount := 2
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: amount,
		},
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicOnePoolHalfFitPodScaleout tests scale out of one pool using HalfBerry pods that half-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfFitPodScaleout(t *testing.T) {
	amount := 4
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryHalfBerry: amount,
		},
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicOnePoolHalfAndFullFitPodScaleout tests scale out of one pool using both HalfBerry and Berry pods that half-fit
// and full-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfAndFullFitPodScaleout(t *testing.T) {
	amount := 4
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryHalfBerry: amount,
			samples.ResourceCategoryBerry:     amount,
		},
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicTwoPoolFullFitPodScaleOut tests the scale-out scenarios for basic variant with 2 pools, where there is only one node template for each pool
// and where any unscheduled pod nearly fully fits into the node template.
func TestBasicTwoPoolFullFitPodScaleOut(t *testing.T) {
	amount := 3
	testData, ok := createTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicTwo,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: amount,
			samples.ResourceCategoryGrape: amount,
		},
		AdviceGenerationMode: commontypes.ScalingAdviceGenerationModeAllAtOnce,
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
	gotPlan := obtainScaleOutPlan(t, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func assertExactScaleOutPlan(t *testing.T, want, got *sacorev1alpha1.ScaleOutPlan) {
	if got == nil {
		t.Fatalf("got nil ScaleOutPlan, want not nil ScaleOutPlan")
		return
	}
	slices.SortFunc(want.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	slices.SortFunc(got.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ScaleOutPlan mismatch (-want +got):\n%s", diff)
	}
}

// createTestData creates the TestData for the given TestArgs.
func createTestData(t *testing.T, args TestArgs) (testData TestData, ok bool) {
	if len(args.NumUnscheduledPerResourceCategory) == 0 {
		t.Fatal("args.NumUnscheduledPerResourceCategory mandatory")
		return
	}
	var err error
	testData.RunContext, testData.Planner, ok = createTestScalingPlanner(t, args.Timeout)
	if !ok {
		return
	}
	testData.Request.CreationTime = time.Now()
	testData.Request.DiagnosticVerbosity = defaultTestVerbosity
	testData.Request.ID = t.Name()
	if args.NodeScoringStrategy != "" {
		testData.Request.ScoringStrategy = args.NodeScoringStrategy
	} else {
		testData.Request.ScoringStrategy = commontypes.NodeScoringStrategyLeastCost
	}
	if args.SimulatorStrategy != "" {
		testData.Request.SimulatorStrategy = args.SimulatorStrategy
	} else {
		testData.Request.SimulatorStrategy = commontypes.SimulatorStrategySingleNodeMultiSim
	}
	if args.AdviceGenerationMode != "" {
		testData.Request.AdviceGenerationMode = args.AdviceGenerationMode
	} else {
		testData.Request.AdviceGenerationMode = commontypes.ScalingAdviceGenerationModeAllAtOnce
	}
	testData.Request.Constraint, err = samples.LoadBasicScalingConstraints(args.PoolCategory)
	if err != nil {
		t.Fatal(err)
		return
	}
	var pods []corev1.Pod
	for c, n := range args.NumUnscheduledPerResourceCategory {
		pods, _, err = samples.GenerateSimplePodsForResourceCategory(c, n, samples.SimplePodMetadata{
			Name: string(c),
		})
		if err != nil {
			t.Fatalf("failed to generate simple pods for resource category %s: %v", c, err)
			return
		}
		testData.Request.Snapshot.Pods = append(testData.Request.Snapshot.Pods, podutil.PodInfosFromCoreV1Pods(pods)...)
	}
	for _, pool := range testData.Request.Constraint.Spec.NodePools {
		for _, nt := range pool.NodeTemplates {
			for _, az := range pool.AvailabilityZones {
				testData.NodePlacements = append(testData.NodePlacements, sacorev1alpha1.NodePlacement{
					NodePoolName:     pool.Name,
					NodeTemplateName: nt.Name,
					InstanceType:     nt.InstanceType,
					Region:           pool.Region,
					AvailabilityZone: az,
				})
			}
		}
	}
	ok = true
	return
}

func createTestScalingPlanner(t *testing.T, duration time.Duration) (runCtx context.Context, planner plannerapi.ScalingPlanner, ok bool) {
	var err error
	defer func() {
		if err != nil {
			ok = false
			t.Errorf("failed to create test planner for test %q: %v", t.Name(), err)
			return
		}
	}()
	if duration == 0 {
		duration = defaultPlannerTimeout
	}
	runCtx = testutil.NewTestContext(t, duration, defaultTestVerbosity)
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessForTop20AWSInstanceTypes()
	if err != nil {
		return
	}
	weightsFn := weights.GetDefaultWeightsFn()
	viewAccess, err := view.NewAccess(runCtx, &minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: minkapi.DefaultWatchQueueSize,
			Timeout:   minkapi.DefaultWatchTimeout,
		},
	})
	if err != nil {
		return
	}

	schedulerConfigBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		return
	}
	simulatorConfig := plannerapi.SimulatorConfig{
		MaxParallelSimulations: plannerapi.DefaultMaxParallelSimulations,
		TrackPollInterval:      plannerapi.DefaultTrackPollInterval,
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		return
	}

	scalePlannerArgs := plannerapi.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   weightsFn,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		SimulatorConfig:   simulatorConfig,
	}
	planner, ok = New(scalePlannerArgs), true
	return
}

func obtainScaleOutPlan(t *testing.T, testData *TestData) *sacorev1alpha1.ScaleOutPlan {
	resultCh := testData.Planner.Plan(testData.RunContext, testData.Request)
	result := <-resultCh
	if result.Error != nil {
		t.Fatalf("failed to generate scale-out plan: %v", result.Error)
		return nil
	} else {
		planResultJson, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatalf("failed to marshal ScalingPlanResult: %v", err)
			return nil
		}
		t.Logf("Obtained ScalingPlanResult %s", planResultJson)
		return result.ScaleOutPlan
	}
}
