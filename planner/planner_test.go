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
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
)

func TestGenerateBasicScalingAdvice(t *testing.T) {
	testCtx, cancelFn := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancelFn()
	runCtx, runCancelFn := commoncli.CreateAppContext(testCtx)
	defer runCancelFn()
	p, err := createTestScalingPlanner(runCtx)
	if err != nil {
		t.Errorf("failed to create test planner: %v", err)
		return
	}

	constraints, err := samples.LoadClusterConstraints(samples.CategoryBasic)
	if err != nil {
		t.Errorf("failed to load basic cluster constraints: %v", err)
		return
	}
	snapshot, err := samples.LoadClusterSnapshot(samples.CategoryBasic)
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
