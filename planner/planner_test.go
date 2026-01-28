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

const defaultVerbosity = 3

func TestOnePoolBasicScenarioWithUnitScaling(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, "one-pool-unit-scaling", time.Minute*15)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	pPool := constraints.Spec.NodePools[0]
	pPoolPlacement := sacorev1alpha1.NodePlacement{
		NodePoolName:     pPool.Name,
		NodeTemplateName: pPool.NodeTemplates[0].Name,
		InstanceType:     pPool.NodeTemplates[0].InstanceType,
		Region:           pPool.Region,
		AvailabilityZone: pPool.AvailabilityZones[0],
	}
	req := createScalingAdviceRequest(t, constraints, snapshot, commontypes.SimulationStrategyMultiSimulationsPerGroup, commontypes.NodeScoringStrategyLeastCost, commontypes.ScalingAdviceGenerationModeAllAtOnce)
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
	// TODO: Add sub-tests for different simulator, generation mode and scoring strategy later
	gotPlan := getScaleOutPlan(ctx, p, req, t)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

func TestOnePoolBasicScenarioWithMultiScaling(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, "one-pool-multi-scaling", time.Second*20)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	req := createScalingAdviceRequest(t, constraints, snapshot, commontypes.SimulationStrategyMultiSimulationsPerGroup, commontypes.NodeScoringStrategyLeastCost, commontypes.ScalingAdviceGenerationModeAllAtOnce)
	if err := samples.IncreaseUnscheduledWorkLoad(req.Snapshot, 2); err != nil {
		t.Error(err)
		return
	}
	pPool := constraints.Spec.NodePools[0]
	pPoolPlacement := sacorev1alpha1.NodePlacement{
		NodePoolName:     pPool.Name,
		NodeTemplateName: pPool.NodeTemplates[0].Name,
		InstanceType:     pPool.NodeTemplates[0].InstanceType,
		Region:           pPool.Region,
		AvailabilityZone: pPool.AvailabilityZones[0],
	}
	wantPlan := &sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: nil,
		Items: []sacorev1alpha1.ScaleOutItem{
			{
				NodePlacement:   pPoolPlacement,
				CurrentReplicas: 1,
				Delta:           2,
			},
		},
	}
	gotPlan := getScaleOutPlan(ctx, p, req, t)
	assertExactScaleOutPlan(wantPlan, gotPlan, t)
}

// TestTwoPoolBasicScaleOutScenarios tests the scale-out scenarios for basic variant with 2 pools.
func TestTwoPoolBasicScaleOutScenarios(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, "two-pool-scale-out", time.Minute*10)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityTwo)
	if !ok {
		return
	}
	req := createScalingAdviceRequest(t, constraints, snapshot, commontypes.SimulationStrategyMultiSimulationsPerGroup, commontypes.NodeScoringStrategyLeastCost, commontypes.ScalingAdviceGenerationModeAllAtOnce)
	pPool := constraints.Spec.NodePools[0]
	qPool := constraints.Spec.NodePools[1]
	pPoolPlacement := sacorev1alpha1.NodePlacement{
		NodePoolName:     pPool.Name,
		NodeTemplateName: pPool.NodeTemplates[0].Name,
		InstanceType:     pPool.NodeTemplates[0].InstanceType,
		Region:           pPool.Region,
		AvailabilityZone: pPool.AvailabilityZones[0],
	}
	qPoolPlacement := sacorev1alpha1.NodePlacement{
		NodePoolName:     qPool.Name,
		NodeTemplateName: qPool.NodeTemplates[0].Name,
		InstanceType:     qPool.NodeTemplates[0].InstanceType,
		Region:           qPool.Region,
		AvailabilityZone: qPool.AvailabilityZones[0],
	}
	t.Run("With1PNodeAnd2BerryPlus1GrapePods", func(t *testing.T) {
		req.ID = t.Name()
		wantPlan := &sacorev1alpha1.ScaleOutPlan{
			UnsatisfiedPodNames: nil,
			Items: []sacorev1alpha1.ScaleOutItem{
				{
					NodePlacement:   pPoolPlacement,
					CurrentReplicas: 1,
					Delta:           1,
				},
				{
					NodePlacement:   qPoolPlacement,
					CurrentReplicas: 0,
					Delta:           1,
				},
			},
		}
		gotPlan := getScaleOutPlan(ctx, p, req, t)
		assertExactScaleOutPlan(wantPlan, gotPlan, t)
	})
	t.Run("With1PNodeAnd3BerryPlus2GrapePods", func(t *testing.T) {
		req.ID = t.Name()
		wantPlan := &sacorev1alpha1.ScaleOutPlan{
			UnsatisfiedPodNames: nil,
			Items: []sacorev1alpha1.ScaleOutItem{
				{
					NodePlacement:   pPoolPlacement,
					CurrentReplicas: 1,
					Delta:           2,
				},
				{
					NodePlacement:   qPoolPlacement,
					CurrentReplicas: 0,
					Delta:           2,
				},
			},
		}
		req, ok := increaseUnscheduledWorkload(req, 1, t)
		if !ok {
			return
		}
		gotPlan := getScaleOutPlan(ctx, p, req, t)
		assertExactScaleOutPlan(wantPlan, gotPlan, t)
	})
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
	logScaleOutPlan(got, t)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ScaleOutPlan mismatch (-want +got):\n%s", diff)
	}
}

func createScalingAdviceRequest(t *testing.T,
	constraints *sacorev1alpha1.ScalingConstraint,
	snapshot *plannerapi.ClusterSnapshot,
	simulationStrategy commontypes.SimulationStrategy,
	scoringStrategy commontypes.NodeScoringStrategy,
	generationMode commontypes.ScalingAdviceGenerationMode) plannerapi.ScalingAdviceRequest {
	return plannerapi.ScalingAdviceRequest{
		ScalingAdviceRequestRef: plannerapi.ScalingAdviceRequestRef{
			ID:            t.Name(),
			CorrelationID: t.Name(),
		},
		Constraint:           constraints,
		Snapshot:             snapshot,
		DiagnosticVerbosity:  defaultVerbosity,
		SimulationStrategy:   simulationStrategy,
		ScoringStrategy:      scoringStrategy,
		AdviceGenerationMode: generationMode,
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

func logScaleOutPlan(scaleOutPlan *sacorev1alpha1.ScaleOutPlan, t *testing.T) bool {
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

func getScaleOutPlan(ctx context.Context, p plannerapi.ScalingPlanner, req plannerapi.ScalingAdviceRequest, t *testing.T) *sacorev1alpha1.ScaleOutPlan {
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
