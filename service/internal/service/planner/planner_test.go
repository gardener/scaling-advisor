package planner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/gardener/scaling-advisor/service/internal/service/simulator/multi"
	"github.com/gardener/scaling-advisor/service/internal/service/weights"
	"github.com/gardener/scaling-advisor/service/internal/testutil"
	pricingtestutil "github.com/gardener/scaling-advisor/service/pricing/testutil"
	"github.com/gardener/scaling-advisor/service/scorer"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
)

func TestGenerateBasicScalingAdvice(t *testing.T) {
	runCtx := commontestutil.LoggerContext(t.Context())
	g, err := createTestGenerator(runCtx)
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

	req := svcapi.ScalingAdviceRequest{
		ScalingAdviceRequestRef: svcapi.ScalingAdviceRequestRef{
			ID:            t.Name(),
			CorrelationID: t.Name(),
		},
		Constraint:        constraints,
		Snapshot:          snapshot,
		EnableDiagnostics: true,
	}

	resultCh := make(chan svcapi.ScalingAdviceResult, 1)
	runArgs := RunArgs{
		Request:   req,
		ResultsCh: resultCh,
	}
	runCtx, cancel := context.WithCancel(runCtx)
	defer cancel()
	g.Run(runCtx, &runArgs)
	adv := <-resultCh
	if adv.Err != nil {
		t.Errorf("failed to generate scaling advice: %v", adv.Err)
		return
	}
	if adv.Response.Diagnostics == nil {
		t.Errorf("expected diagnostics to be set, got nil")
		return
	}
	scalingAdvice := adv.Response.ScalingAdvice
	scalingAdviceBytes, err := json.Marshal(scalingAdvice)
	if err != nil {
		t.Errorf("failed to marshal scaling advice: %v", err)
		return
	}
	t.Logf("generated scaling advice: %+v", string(scalingAdviceBytes))

	if len(scalingAdvice.Spec.ScaleOutPlan.Items) != 1 {
		t.Errorf("expected 1 scale out item, got %d", len(scalingAdvice.Spec.ScaleOutPlan.Items))
		return
	}
	if scalingAdvice.Spec.ScaleOutPlan.Items[0].Delta != 1 {
		t.Errorf("expected scale out delta of 1, got %d", scalingAdvice.Spec.ScaleOutPlan.Items[0].Delta)
		return
	}
	if scalingAdvice.Spec.ScaleOutPlan.Items[0].NodeTemplateName != constraints.Spec.NodePools[0].NodeTemplates[0].Name {
		t.Errorf("expected node template name %q, got %q", constraints.Spec.NodePools[0].NodeTemplates[0].Name, scalingAdvice.Spec.ScaleOutPlan.Items[0].NodeTemplateName)
		return
	}
}

func createTestGenerator(ctx context.Context) (*Planner, error) {
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessForTop20AWSInstanceTypes()
	if err != nil {
		return nil, err
	}
	weightsFn := weights.GetDefaultWeightsFn()
	nodeScorer, err := scorer.GetNodeScorer(commontypes.NodeScoringStrategyLeastCost, pricingAccess, weightsFn)
	if err != nil {
		return nil, err
	}
	nodeSelector, err := scorer.GetNodeScoreSelector(commontypes.NodeScoringStrategyLeastCost)
	if err != nil {
		return nil, err
	}
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
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, 1)
	if err != nil {
		return nil, err
	}

	args := Args{
		ViewAccess:        viewAccess,
		PricingAccess:     pricingAccess,
		ResourceWeigher:   weightsFn,
		NodeScorer:        nodeScorer,
		Selector:          nodeSelector,
		SimulationCreator: svcapi.SimulationCreatorFunc(multi.NewGroup),
		SimulationGrouper: svcapi.SimulationGrouperFunc(multi.createSimulationGroups),
		SchedulerLauncher: schedulerLauncher,
	}

	return New(&args), nil
}
