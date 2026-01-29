// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"encoding/json"
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
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
	"github.com/google/go-cmp/cmp"
)

const defaultVerbosity = 2

func Test1PoolBasicUnitScaleOut(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, t.Name(), time.Second*10)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	pPoolPlacement := placementsForFirstTemplateAndFirstAvailabilityZone(constraints.Spec.NodePools)[0]
	req := requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   pPoolPlacement,
				CurrentReplicas: 1,
				Delta:           1,
			},
		},
	}
	gotPlan := getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

func Test1PoolBasicMultiScaleout(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, t.Name(), time.Minute*20)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	pPoolPlacement := placementsForFirstTemplateAndFirstAvailabilityZone(constraints.Spec.NodePools)[0]
	req := requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	numExtraBerryPods := 2
	req, ok = increaseUnscheduledWorkload(req, numExtraBerries, t)
	if !ok {
		return
	}
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   pPoolPlacement,
				CurrentReplicas: 1,
				Delta:           int32(numExtraBerries + 1),
			},
		},
	}
	gotPlan := getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

// Test2PoolBasicUnitScaleOut tests the scale-out scenarios for basic variant with 2 pools, unit scaling each pool..
func Test2PoolBasicUnitScaleOut(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, t.Name(), time.Second*10)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityTwo)
	if !ok {
		return
	}
	req := requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	placements := placementsForFirstTemplateAndFirstAvailabilityZone(constraints.Spec.NodePools)
	pPlacement, qPlacement := placements[0], placements[1]
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   pPlacement,
				CurrentReplicas: 1,
				Delta:           1,
			},
			{
				NodePlacement:   qPlacement,
				CurrentReplicas: 0,
				Delta:           1,
			},
		},
	}
	gotPlan := getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

// Test2PoolBasicMultiScaleout tests the basic variant of the scale-out scenario for 2 pools, with more than one scaling for each pool.
func Test2PoolBasicMultiScaleout(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, t.Name(), time.Second*30)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityTwo)
	if !ok {
		return
	}
	req := requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	placements := placementsForFirstTemplateAndFirstAvailabilityZone(constraints.Spec.NodePools)
	unscheduledUnitIncrease := 1
	req, ok = increaseUnscheduledWorkload(req, unscheduledUnitIncrease, t)
	if !ok {
		return
	}
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   placements[0],
				CurrentReplicas: 1,
				Delta:           int32(1 + unscheduledUnitIncrease),
			},
			{
				NodePlacement:   placements[1],
				CurrentReplicas: 0,
				Delta:           int32(1 + unscheduledUnitIncrease),
			},
		},
	}
	gotPlan := getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

func TestReusePlannerAcrossRequests(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, t.Name(), time.Second*10)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	pPoolPlacement := placementsForFirstTemplateAndFirstAvailabilityZone(constraints.Spec.NodePools)[0]
	req := requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	req.ID = "TestReusePlannerAcrossRequests-A"
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   pPoolPlacement,
				CurrentReplicas: 1,
				Delta:           1,
			},
		},
	}
	gotPlan := getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)

	req = requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t, constraints, snapshot)
	req.ID = "TestReusePlannerAcrossRequests-B"
	gotPlan = getScaleOutPlan(t, ctx, p, req)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

func placementsForFirstTemplateAndFirstAvailabilityZone(pools []sacorev1alpha1.NodePool) []sacorev1alpha1.NodePlacement {
	placements := make([]sacorev1alpha1.NodePlacement, 0, len(pools))
	for _, pool := range pools {
		placements = append(placements, sacorev1alpha1.NodePlacement{
			NodePoolName:     pool.Name,
			NodeTemplateName: pool.NodeTemplates[0].Name,
			InstanceType:     pool.NodeTemplates[0].InstanceType,
			Region:           pool.Region,
			AvailabilityZone: pool.AvailabilityZones[0],
		})
	}
	return placements
}

func increaseUnscheduledWorkload(in plannerapi.ScalingAdviceRequest, amount int, t *testing.T) (out plannerapi.ScalingAdviceRequest, ok bool) {
	out = in
	out.Snapshot.Pods = slices.Clone(in.Snapshot.Pods)
	err := samples.IncreaseUnscheduledWorkLoad(out.Snapshot, amount)
	if err != nil {
		t.Error(err)
		return
	}
	ok = true
	return
}

func assertExactScaleOutPlan(want, got *sacorev1alpha1.ScaleOutPlan, t *testing.T) {
	if got == nil {
		t.Fatalf("got nil ScaleOutPlan, want not nil ScaleOutPlan")
	}
	slices.SortFunc(want.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	slices.SortFunc(got.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	if !logScaleOutPlan(t, got) {
		return
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ScaleOutPlan mismatch (-want +got):\n%s", diff)
	}
}

func requestForAllAtOnceAdviceWithLeastCostMultiSimulationStrategy(t *testing.T,
	constraints *sacorev1alpha1.ScalingConstraint,
	snapshot *plannerapi.ClusterSnapshot) plannerapi.ScalingAdviceRequest {
	return plannerapi.ScalingAdviceRequest{
		CreationTime: time.Now(),
		ScalingAdviceRequestRef: plannerapi.ScalingAdviceRequestRef{
			ID:            t.Name(),
			CorrelationID: t.Name(),
		},
		Constraint:           constraints,
		Snapshot:             snapshot,
		DiagnosticVerbosity:  defaultVerbosity,
		SimulationStrategy:   commontypes.SimulationStrategyMultiSimulationsPerGroup,
		ScoringStrategy:      commontypes.NodeScoringStrategyLeastCost,
		AdviceGenerationMode: commontypes.ScalingAdviceGenerationModeAllAtOnce,
	}
}

func createRunContext(t *testing.T, name string, duration time.Duration) context.Context {
	t.Helper()
	testCtx, cancelFn := context.WithTimeout(t.Context(), duration)
	t.Cleanup(cancelFn) // enough — no need to clean up the child cancel func separately
	runCtx, _ := commoncli.CreateAppContext(testCtx, name)
	return runCtx
}

func loadBasicConstraintsAndSnapshot(t *testing.T, poolCardinality samples.PoolCardinality) (constraints *sacorev1alpha1.ScalingConstraint, snapshot *plannerapi.ClusterSnapshot, ok bool) {
	constraints, err := samples.LoadBasicScalingConstraints(poolCardinality)
	if err != nil {
		t.Error(err)
		return
	}
	snapshot, err = samples.LoadBasicClusterSnapshot(poolCardinality)
	if err != nil {
		t.Errorf("failed to load basic cluster snapshot for poolCardinality %q: %v", poolCardinality, err)
		return
	}
	ok = true
	return
}

func logScaleOutPlan(t *testing.T, scaleOutPlan *sacorev1alpha1.ScaleOutPlan) bool {
	t.Helper()
	if scaleOutPlan == nil {
		return false
	}
	scaleOutPlanBytes, err := json.Marshal(scaleOutPlan)
	if err != nil {
		t.Errorf("failed to marshal scale-out plan: %v", err)
		return false
	}
	t.Logf("produced scale-out plan: %+v", string(scaleOutPlanBytes))
	return true
}

func createScalingPlanner(t *testing.T, testName string, duration time.Duration) (runCtx context.Context, planner plannerapi.ScalingPlanner, ok bool) {
	var err error
	defer func() {
		if err != nil {
			ok = false
			t.Errorf("failed to create test planner for test %q: %v", testName, err)
			return
		}
	}()
	runCtx = createRunContext(t, testName, duration)
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

func getScaleOutPlan(t *testing.T, ctx context.Context, p plannerapi.ScalingPlanner, req plannerapi.ScalingAdviceRequest) *sacorev1alpha1.ScaleOutPlan {
	resultCh := make(chan plannerapi.ScalingPlanResult, 1)
	defer close(resultCh)
	p.Plan(ctx, req, resultCh)
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("failed to generate scale-out plan: %v", result.Err)
		return nil
	} else {
		return result.ScaleOutPlan
	}
}
