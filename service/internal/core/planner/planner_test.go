package planner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/service"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/service/internal/core/weights"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/gardener/scaling-advisor/service/internal/testutil"
	pricingtestutil "github.com/gardener/scaling-advisor/service/pricing/testutil"
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

	constraints, err := testutil.LoadBasicClusterConstraints(testutil.BasicCluster)
	if err != nil {
		t.Errorf("failed to load basic cluster constraints: %v", err)
		return
	}
	snapshot, err := testutil.LoadBasicClusterSnapshot(testutil.BasicCluster)
	if err != nil {
		t.Errorf("failed to load basic cluster snapshot: %v", err)
		return
	}

	req := service.ScalingAdviceRequest{
		ScalingAdviceRequestRef: service.ScalingAdviceRequestRef{
			ID:            t.Name(),
			CorrelationID: t.Name(),
		},
		Constraint:          constraints,
		Snapshot:            snapshot,
		DiagnosticVerbosity: 2,
		ScoringStrategy:     commontypes.NodeScoringStrategyLeastCost,
		SimulationStrategy:  commontypes.SimulationStrategyMultiSimulationsPerGroup,
	}

	resultCh := make(chan service.ScalingPlanResult, 1)
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

func createTestScalingPlanner(ctx context.Context) (service.ScalingPlanner, error) {
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

	schedulerConfigBytes, err := testutil.ReadSchedulerConfig()
	if err != nil {
		return nil, err
	}
	simulatorConfig := service.SimulatorConfig{
		MaxParallelSimulations: service.DefaultMaxParallelSimulations,
		TrackPollInterval:      service.DefaultTrackPollInterval,
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		return nil, err
	}

	scalePlannerArgs := service.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   weightsFn,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		SimulatorConfig:   simulatorConfig,
	}

	return New(scalePlannerArgs), nil
}
