// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"encoding/json"
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
)

func TestOnePoolBasicScenarioWithUnitScaling(t *testing.T) {
	ctx, p, ok := createScalingPlanner(t, "one-pool-unit-scaling", time.Second*30)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityOne)
	if !ok {
		return
	}
	req := createScalingAdviceRequest(t, constraints, snapshot, commontypes.SimulationStrategyMultiSimulationsPerGroup, commontypes.NodeScoringStrategyLeastCost, commontypes.ScalingAdviceGenerationModeAllAtOnce)
	// TODO: Add sub-tests for different simulator, generation mode and scoring strategy later
	planResult := getScalingPlanResult(ctx, p, req)
	if !assertScaleOutPlan(t, constraints, planResult, 1, 1) {
		return
	}
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
	planResult := getScalingPlanResult(ctx, p, req)
	if !assertScaleOutPlan(t, constraints, planResult, 1, 2) {
		return
	}
}

// TestTwoPoolBasicScenario tests the basic variant of the scaling scenario with 2 pools.
func TestTwoPoolBasicScenario(t *testing.T) {
	//ctx, p, ok := createScalingPlanner(t, "two-pool-test", time.Second*20)
	ctx, p, ok := createScalingPlanner(t, "two-pool-test", time.Minute*10)
	if !ok {
		return
	}
	constraints, snapshot, ok := loadBasicConstraintsAndSnapshot(t, samples.PoolCardinalityTwo)
	if !ok {
		return
	}
	req := createScalingAdviceRequest(t, constraints, snapshot, commontypes.SimulationStrategyMultiSimulationsPerGroup, commontypes.NodeScoringStrategyLeastCost, commontypes.ScalingAdviceGenerationModeAllAtOnce)
	req.DiagnosticVerbosity = 6

	t.Run("1PNodeWith2BerryPodAnd1GrapePod", func(t *testing.T) {
		planResult := getScalingPlanResult(ctx, p, req)
		if planResult.Err != nil {
			t.Error(planResult.Err)
			return
		}
		logScaleOutPlan(t, planResult.ScaleOutPlan)
	})
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
		DiagnosticVerbosity:  1,
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

func assertScaleOutPlan(t *testing.T, constraints *sacorev1alpha1.ScalingConstraint, planResult plannerapi.ScalingPlanResult, wantScaleOutItems int, wantDelta int32) bool {
	if planResult.Err != nil {
		t.Errorf("failed to generate scaling plan result: %v", planResult.Err)
		return false
	}
	got := planResult.ScaleOutPlan
	if got == nil {
		t.Errorf("expected scale-out plan to be set, got nil")
		return false
	}
	if !logScaleOutPlan(t, got) {
		return false
	}
	if len(got.Items) != wantScaleOutItems {
		t.Errorf("expected 1 scale-out item, got %d", len(got.Items))
		return false
	}
	if got.Items[0].Delta != wantDelta {
		t.Errorf("expected scale-out delta of 1, got %d", got.Items[0].Delta)
		return false
	}
	if got.Items[0].NodeTemplateName != constraints.Spec.NodePools[0].NodeTemplates[0].Name {
		t.Errorf("expected node template name %q, got %q", constraints.Spec.NodePools[0].NodeTemplates[0].Name, got.Items[0].NodeTemplateName)
		return false
	}
	return true
}

func logScaleOutPlan(t *testing.T, scaleOutPlan *sacorev1alpha1.ScaleOutPlan) bool {
	t.Helper()
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

func getScalingPlanResult(ctx context.Context, p plannerapi.ScalingPlanner, req plannerapi.ScalingAdviceRequest) (result plannerapi.ScalingPlanResult) {
	resultCh := make(chan plannerapi.ScalingPlanResult, 1)
	defer close(resultCh)
	p.Plan(ctx, req, resultCh)
	result = <-resultCh
	return
}
