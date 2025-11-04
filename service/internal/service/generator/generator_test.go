package generator

import (
	"context"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/gardener/scaling-advisor/service/internal/service/simulation"
	"github.com/gardener/scaling-advisor/service/internal/service/weights"
	"github.com/gardener/scaling-advisor/service/internal/testutil"
	pricingtestutil "github.com/gardener/scaling-advisor/service/pricing/testutil"
	"github.com/gardener/scaling-advisor/service/scorer"
	"testing"
)

func TestGenerateBasicScalingAdvise(t *testing.T) {
	g, err := createTestGenerator()
	if err != nil {
		t.Errorf("failed to create test generator: %v", err)
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
		Constraint: constraints,
		Snapshot:   snapshot,
	}

	advEventCh := make(chan svcapi.ScalingAdviceEvent, 1)
	runArgs := RunArgs{
		Request:       req,
		AdviceEventCh: advEventCh,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Run(ctx, &runArgs)
	adv := <-advEventCh
	if adv.Err != nil {
		t.Errorf("failed to generate scaling advice: %v", adv.Err)
		return
	}
	t.Logf("generated scaling advice: %+v", adv.Response)
}

func createTestGenerator() (*Generator, error) {
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessWithFilteredAWSData()
	weightsFn := weights.GetDefaultWeightsFn()
	nodeScorer, err := scorer.GetNodeScorer(commontypes.LeastCostNodeScoringStrategy, pricingAccess, weightsFn)
	if err != nil {
		return nil, err
	}
	nodeSelector, err := scorer.GetNodeScoreSelector(commontypes.LeastCostNodeScoringStrategy)
	if err != nil {
		return nil, err
	}
	viewAccess, err := view.NewAccess(&minkapi.ViewArgs{
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
		WeightsFn:         weightsFn,
		NodeScorer:        nodeScorer,
		Selector:          nodeSelector,
		SimulationCreator: svcapi.SimulationCreatorFunc(simulation.New),
		SimulationGrouper: svcapi.SimulationGrouperFunc(simulation.CreateSimulationGroups),
		SchedulerLauncher: schedulerLauncher,
	}

	return New(&args), nil
}
