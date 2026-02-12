// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"encoding/json"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/planner/scheduler"
	"github.com/gardener/scaling-advisor/planner/weights"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
)

const defaultTestVerbosity = 2

const defaultPlannerTimeout = 30 * time.Second

// TestArgs represents the common test args for the scale-out unit-tests of the ScalingPlanner
type TestArgs struct {
	NumUnscheduledPerResourceCategory map[samples.ResourceCategory]int
	PoolCategory                      samples.PoolCategory
	SimulatorStrategy                 commontypes.SimulatorStrategy
	NodeScoringStrategy               commontypes.NodeScoringStrategy
	AdviceGenerationMode              commontypes.ScalingAdviceGenerationMode
	Timeout                           time.Duration
	PVCNames                          []string
}

// TestData holds all the common test data necessary for carrying out the scale-out unit-tests of the ScalingPlanner and asserting conditions
type TestData struct {
	RunContext     context.Context
	SnapshotPath   string
	NodePlacements []sacorev1alpha1.NodePlacement
	Request        plannerapi.Request
}

func TestBasicOnePoolUnitScaleOut(t *testing.T) {
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func TestBasicOnePoolScaleOutWithVolumeClaim(t *testing.T) {
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
		PoolCategory: samples.PoolCategoryBasicOne,
		NumUnscheduledPerResourceCategory: map[samples.ResourceCategory]int{
			samples.ResourceCategoryBerry: 1,
		},
		PVCNames: []string{"stem"},
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func TestReusePlannerAcrossRequests(t *testing.T) {
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
	testData.Request.ID = t.Name() + "-B"
	gotPlan = obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

func TestBasicOnePoolFullFitPodScaleout(t *testing.T) {
	amount := 2
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicOnePoolHalfFitPodScaleout tests scale out of one pool using HalfBerry pods that half-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfFitPodScaleout(t *testing.T) {
	amount := 4
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicOnePoolHalfAndFullFitPodScaleout tests scale out of one pool using both HalfBerry and Berry pods that half-fit
// and full-fit into pool P's NodeTemplate.
func TestBasicOnePoolHalfAndFullFitPodScaleout(t *testing.T) {
	amount := 4
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
	assertExactScaleOutPlan(t, wantPlan, gotPlan)
}

// TestBasicTwoPoolFullFitPodScaleOut tests the scale-out scenarios for basic variant with 2 pools, where there is only one node template for each pool
// and where any unscheduled pod nearly fully fits into the node template.
func TestBasicTwoPoolFullFitPodScaleOut(t *testing.T) {
	amount := 3
	planner, testData, ok := createTestPlannerAndTestData(t, TestArgs{
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
	gotPlan := obtainScaleOutPlan(t, planner, &testData)
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

// createPlannerAndTest creates the test ScalingPlanner and TestData for the given TestArgs.
func createTestPlannerAndTestData(t *testing.T, args TestArgs) (planner plannerapi.ScalingPlanner, testData TestData, ok bool) {
	if len(args.NumUnscheduledPerResourceCategory) == 0 {
		t.Fatal("args.NumUnscheduledPerResourceCategory mandatory")
		return
	}
	var err error
	testData.RunContext, planner, ok = createTestScalingPlanner(t, args.Timeout)
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
		t.Errorf("failed to create test planner: %v", err)
		return
	}
	var pods []corev1.Pod
	for c, n := range args.NumUnscheduledPerResourceCategory {
		pods, _, err = samples.GenerateSimplePodsForResourceCategory(c, n, samples.SimplePodGenInput{
			Name:          string(c),
			SchedulerName: "bin-packing-scheduler",
			PVCNames:      args.PVCNames,
		})
		if err != nil {
			t.Fatalf("failed to generate simple pods for resource category %s: %v", c, err)
			return
		}
		testData.Request.Snapshot.Pods = append(testData.Request.Snapshot.Pods, podutil.PodInfosFromCoreV1Pods(pods)...)
	}
	if len(args.PVCNames) > 0 {
		_, _, err = samples.GenerateStorageClass(commontypes.CloudProviderAWS, "default", storagev1.VolumeBindingWaitForFirstConsumer)
		if err != nil {
			t.Fatalf("failed to generate storage class %q: %v", "default", err)
			return
		}
		volNs := corev1.NamespaceDefault
		volStorage := resource.MustParse("1Gi")
		_, _, err = samples.GeneratePersistentVolumeClaims(volNs, volStorage, corev1.ReadWriteMany, args.PVCNames)
		if err != nil {
			t.Fatalf("failed to generate pvcs: %v", err)
			return
		}
		_, _, err = samples.GeneratePersistentVolumes(samples.SimplePVGenInput{
			Storage:    volStorage,
			AccessMode: corev1.ReadWriteMany,
			Zone:       testData.Request.Constraint.Spec.NodePools[0].AvailabilityZones[0],
			PVCNames:   args.PVCNames,
		})
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
		t.Errorf("failed to load basic cluster snapshot: %v", err)
		return
	}

	req := plannerapi.ScalingAdviceRequest{
		ScalingAdviceRequestRef: plannerapi.ScalingAdviceRequestRef{
			ID:            t.Name(),
			CorrelationID: t.Name(),
		},
		Constraint:           constraints,
		Snapshot:             snapshot,
		DiagnosticVerbosity:  2,
		ScoringStrategy:      commontypes.NodeScoringStrategyLeastCost,
		SimulationStrategy:   commontypes.SimulationStrategyMultiSimulationsPerGroup,
		AdviceGenerationMode: commontypes.ScalingAdviceGenerationModeAllAtOnce,
	}

	resultCh := make(chan plannerapi.ScalingPlanResult, 1)
	defer close(resultCh)
	p.Plan(runCtx, req, resultCh)
	planResult := <-resultCh
	if planResult.Err != nil {
		t.Errorf("failed to produce plan result: %v", planResult.Err)
		return
	}
	//if planResult.Response.Diagnostics == nil {
	//	t.Errorf("expected diagnostics to be set, got nil")
	//	return
	//}
	scaleOutPlan := planResult.ScaleOutPlan
	if scaleOutPlan == nil {
		t.Errorf("expected scale-out plan to be set, got nil")
		return
	}
	scaleOutPlanBytes, err := json.Marshal(scaleOutPlan)
	if err != nil {
		t.Errorf("failed to marshal scale-out plan: %v", err)
		return
	}
	t.Logf("produced scale-out plan: %+v", string(scaleOutPlanBytes))

	if len(scaleOutPlan.Items) != 1 {
		t.Errorf("expected 1 scale-out item, got %d", len(scaleOutPlan.Items))
		return
	}
	if scaleOutPlan.Items[0].Delta != 1 {
		t.Errorf("expected scale-out delta of 1, got %d", scaleOutPlan.Items[0].Delta)
		return
	}
	if scaleOutPlan.Items[0].NodeTemplateName != constraints.Spec.NodePools[0].NodeTemplates[0].Name {
		t.Errorf("expected node template name %q, got %q", constraints.Spec.NodePools[0].NodeTemplates[0].Name, scaleOutPlan.Items[0].NodeTemplateName)
		return
	}
}

func createTestScalingPlanner(ctx context.Context) (plannerapi.ScalingPlanner, error) {
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessForTop20AWSInstanceTypes()
	if err != nil {
		return nil, err
	}
	weightsFn := weights.GetDefaultWeightsFn()
	viewAccess, err := view.NewAccess(ctx, &minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: minkapi.DefaultWatchQueueSize,
			Timeout:   minkapi.DefaultWatchTimeout,
		},
	})
	if err != nil {
		return nil, err
	}

	schedulerConfigBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		return nil, err
	}
	simulatorConfig := plannerapi.SimulatorConfig{
		MaxParallelSimulations: plannerapi.DefaultMaxParallelSimulations,
		TrackPollInterval:      plannerapi.DefaultTrackPollInterval,
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		return nil, err
	}

	scalePlannerArgs := plannerapi.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   weightsFn,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		SimulatorConfig:   simulatorConfig,
	}

	return New(scalePlannerArgs), nil
}
