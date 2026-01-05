package multi

import (
	"context"
	"fmt"
	"sync/atomic"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/service/internal/core/simulator"
	"github.com/gardener/scaling-advisor/service/internal/core/util"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

var _ service.ScaleOutSimulator = (*multiSimulator)(nil)

// TODO find a better word for multiSimulator.
type multiSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher service.SchedulerLauncher
	nodeScorer        service.NodeScorer
	simulatorConfig   service.SimulatorConfig
	// simulationCreator is a factory interface to create Simulations. This allows testing either the ScalingPlanner or MultiSimulator
	// with mock simulations.
	simulationCreator    service.SimulationCreator
	simulationRunCounter atomic.Uint32
	request              *service.ScalingAdviceRequest
}

// NewScaleOutSimulator creates a new service.ScaleOutSimulator that runs multiple simulations concurrently.
func NewScaleOutSimulator(viewAccess minkapi.ViewAccess, schedulerLauncher service.SchedulerLauncher, nodeScorer service.NodeScorer, simulatorConfig service.SimulatorConfig, req *service.ScalingAdviceRequest) (service.ScaleOutSimulator, error) {
	return &multiSimulator{
		viewAccess:        viewAccess,
		schedulerLauncher: schedulerLauncher,
		nodeScorer:        nodeScorer,
		simulatorConfig:   simulatorConfig,
		simulationCreator: service.SimulationCreatorFunc(NewSimulation),
		request:           req,
	}, nil
}

func (m *multiSimulator) Simulate(ctx context.Context, resultCh chan<- service.ScalingPlanResult) {
	var err error
	defer func() {
		if err != nil {
			util.SendPlanError(resultCh, m.request.GetRef(), err)
		}
	}()
	baseView := m.viewAccess.GetBaseView()
	if err = simulator.SynchronizeBaseView(ctx, baseView, m.request.Snapshot); err != nil {
		return
	}

	m.simulationRunCounter.Store(0) // initialize it to 0.
	simulationGroups, err := m.createSimulationGroups(m.request)
	if err != nil {
		return
	}
	err = m.runAllGroups(ctx, baseView, simulationGroups, resultCh)
}

func (m *multiSimulator) createSimulationGroups(request *service.ScalingAdviceRequest) ([]service.SimulationGroup, error) {
	var allSimulations []service.Simulation
	for _, nodePool := range request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				var (
					sim service.Simulation
					err error
				)
				simulationName := fmt.Sprintf("%s_%s_%s", nodePool.Name, nodeTemplate.Name, zone)
				sim, err = m.createSimulation(simulationName, &nodePool, nodeTemplate.Name, zone)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return createSimulationGroups(allSimulations)
}

func (m *multiSimulator) createSimulation(simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string) (service.Simulation, error) {
	simArgs := &service.SimulationArgs{
		RunCounter:        &m.simulationRunCounter,
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: m.schedulerLauncher,
		Config:            m.simulatorConfig,
	}
	return m.simulationCreator.Create(simulationName, simArgs)
}

//func (m *multiSimulator) runPasses(ctx context.Context, groups []core.SimulationGroup, consumeFn minkapi.View) error {
//	ctxLog := logr.FromContextOrDiscard(ctx)
//	var (
//		allWinnerNodeScores     []core.NodeScore
//		leftoverUnscheduledPods []types.NamespacedName
//	)
//runPassesLoop:
//	for {
//		groupRunPassNum := m.simulationRunCounter.Load()
//		log := ctxLog.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
//		select {
//		case <-ctx.Done():
//			err := ctx.Err()
//			log.Error(err, "ScaleOutSimulator context done. Aborting pass runPassesLoop")
//			return err
//		default:
//			passCtx := logr.NewContext(ctx, log)
//			passWinnerNodeScores, passLeftoverUnscheduledPods, err := m.runPass(passCtx, groups)
//			if err != nil {
//				return err
//			}
//			// If there are no winning nodes produced by a pass for the pending unscheduled pods, then abort the runPassesLoop.
//			// This means that we could not identify any node from the node pool and node template combinations (as specified in the constraint)
//			// that could accommodate any unscheduled pods. It is fruitless to continue further.
//			if len(passWinnerNodeScores) == 0 {
//				log.Info("Aborting runPassesLoop since no node scores produced in pass.")
//				break runPassesLoop
//			}
//			allWinnerNodeScores = append(allWinnerNodeScores, passWinnerNodeScores...)
//			if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeIncremental {
//				if err = createAndSendPlanResult(log, m.request, passWinnerNodeScores, passLeftoverUnscheduledPods, consumeFn); err != nil {
//					return err
//				}
//			}
//			if len(passLeftoverUnscheduledPods) == 0 {
//				log.Info("All pods have been scheduled in pass")
//				leftoverUnscheduledPods = passLeftoverUnscheduledPods
//				break runPassesLoop
//			}
//			m.simulationRunCounter.Add(1)
//		}
//	}
//
//	if len(allWinnerNodeScores) == 0 {
//		ctxLog.Info("No scaling advice generated. No winning nodes produced by any simulation group.")
//		return core.ErrNoScalingAdvice
//	}
//
//	if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
//		if err := createAndSendPlanResult(ctxLog, m.request, allWinnerNodeScores, leftoverUnscheduledPods, consumeFn); err != nil {
//			return err
//		}
//	}
//	return nil
//}

// runPass iterates through the simulation groups. For each group, it runs the group specific simulation(s) and
// obtains the core.SimulationGroupResult which encapsulates the results of all simulations in the group.
// It then runs the NodeScorer against the core.SimulationGroupResult and selects a winner amongst all the computed
// core.NodeScore's. If there is no winner node score it will continue to the next core.SimulationGroup.
// If there is a winner node score, it appends to the returned slice of allWinnerNodeScores.
// If there are no leftover unscheduled pods after processing a group, it breaks the loop and returns.
//func (m *multiSimulator) runPass(ctx context.Context, groups []core.SimulationGroup) (allWinnerNodeScores []core.NodeScore, unscheduledPods []types.NamespacedName, err error) {
//	log := logr.FromContextOrDiscard(ctx)
//	var (
//		groupResult core.SimulationGroupResult
//		groupScores core.SimulationGroupScores
//	)
//	for groupIndex := 0; groupIndex < len(groups); {
//		group := groups[groupIndex]
//		groupResult, err = group.Run(ctx)
//		if err != nil {
//			return
//		}
//		groupScores, err = m.processSimulationGroupResults(m.nodeScorer, &groupResult)
//		if err != nil {
//			return
//		}
//		if groupScores.WinnerNodeScore == nil {
//			log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupResult.Name)
//			groupIndex++
//			continue
//		}
//		allWinnerNodeScores = append(allWinnerNodeScores, *groupScores.WinnerNodeScore)
//		unscheduledPods = groupScores.WinnerNodeScore.UnscheduledPods
//		if len(groupScores.WinnerNodeScore.UnscheduledPods) == 0 {
//			log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupResult.Name)
//			break
//		}
//	}
//
//	for _, group := range groups {
//		groupResult, err = group.Run(ctx)
//		if err != nil {
//			return
//		}
//		groupScores, err = m.processSimulationGroupResults(g.args.NodeScorer, &groupResult)
//		if err != nil {
//			return
//		}
//		if groupScores.WinnerNodeScore == nil {
//			log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupResult.Name)
//			continue
//		}
//		allWinnerNodeScores = append(allWinnerNodeScores, *groupScores.WinnerNodeScore)
//		unscheduledPods = groupScores.WinnerNodeScore.UnscheduledPods
//		if len(groupScores.WinnerNodeScore.UnscheduledPods) == 0 {
//			log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupResult.Name)
//			break
//		}
//	}
//	return
//}

// runAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running passes for each group it will obtain the winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
func (m *multiSimulator) runAllGroups(ctx context.Context, baseView minkapi.View, simGroups []service.SimulationGroup, resultCh chan<- service.ScalingPlanResult) (err error) {
	var (
		groupView               = baseView
		groupWinnerNodeScores   []service.NodeScore
		allWinnerNodeScores     []service.NodeScore
		leftoverUnscheduledPods []types.NamespacedName
		simGroupRunResult       util.SimulationGroupRunResult
		log                     = logr.FromContextOrDiscard(ctx)
	)
	for groupIndex := 0; groupIndex < len(simGroups); {
		group := simGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		simGroupRunResult, err = m.runAllPassesForGroup(grpCtx, groupView, group)
		if err != nil {
			err = fmt.Errorf("failed to run all passes for group %q: %w", group.Name(), err)
			return
		}
		if len(groupWinnerNodeScores) == 0 {
			log.Info("No winning node scores produced for group. Continuing to next group.")
			groupIndex++
			continue
		}
		allWinnerNodeScores = append(allWinnerNodeScores, groupWinnerNodeScores...)
		if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeIncremental {
			log.Info("Sending incremental scale-out plan")
			if err = util.SendPlanResult(m.request, simGroupRunResult, resultCh); err != nil {
				return
			}
		}
		if len(leftoverUnscheduledPods) == 0 {
			log.Info("Ending runAllGroups: all pods have been scheduled after processing group")
			break
		}
	}
	if len(allWinnerNodeScores) == 0 {
		log.Info("No winning node scores produced by any pass of all simulation groups.")
		err = service.ErrNoScalingAdvice
		return
	}
	if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
		log.Info("Sending all-at-once scale-out plan")
		err = util.SendPlanResult(m.request, simGroupRunResult, resultCh)
	}
	return
}

// runAllPassesForGroup runs all passes for the given simulation group until there is no winner or there are no leftover unscheduled pods or the context is done.
func (m *multiSimulator) runAllPassesForGroup(ctx context.Context, groupView minkapi.View, group service.SimulationGroup) (sgrr util.SimulationGroupRunResult, err error) {
	var (
		winningNodeScore *service.NodeScore
	)
	sgrr.NextGroupView = groupView
	sgrr.NumPasses = 1 // it will run at least once.
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			log := logr.FromContextOrDiscard(ctx).WithValues("numGroupRunPass", sgrr.NumPasses)
			passCtx := logr.NewContext(ctx, log)
			sgrr.NextGroupView, winningNodeScore, err = m.runSinglePassForGroup(passCtx, sgrr.NextGroupView, group)
			if err != nil {
				return
			}
			// winningNodeScore being nil indicates that there are no more winning node score, further passes can be aborted.
			if winningNodeScore == nil {
				log.Info("No winning node score produced in pass. Ending group passes.")
				return
			}
			sgrr.WinnerNodeScores = append(sgrr.WinnerNodeScores, *winningNodeScore)
			// It captures the leftover unscheduled pods from the last winning node score.
			// If there is no winning node score in the current pass, the leftover unscheduled pods from the
			// previous pass will be retained.
			sgrr.LeftoverUnscheduledPods = winningNodeScore.UnscheduledPods
			if len(sgrr.LeftoverUnscheduledPods) == 0 {
				log.Info("All pods have been scheduled in pass")
				return
			}
		}
		sgrr.NumPasses++
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView.
// If there is a winnerNodeScore among the simulations in the group, it is returned along with the nextGroupView.
// If there is no winner then winner node score is nil and the nextGroupView is nil.
func (m *multiSimulator) runSinglePassForGroup(ctx context.Context, passView minkapi.View, group service.SimulationGroup) (nextPassView minkapi.View, winnerNodeScore *service.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupResult service.SimulationGroupResult
		groupScores service.SimulationGroupScores
		winnerView  minkapi.View
	)
	getSimViewFn := func(ctx context.Context, name string) (minkapi.View, error) {
		return m.viewAccess.GetSandboxViewOverDelegate(ctx, name, passView)
	}
	groupResult, err = group.Run(ctx, getSimViewFn)
	if err != nil {
		return
	}
	groupScores, winnerView, err = m.processSimulationGroupResults(m.nodeScorer, &groupResult)
	if err != nil {
		return
	}
	if groupScores.WinnerNodeScore == nil {
		log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupResult.Name)
		nextPassView = passView
		return
	}
	winnerNodeScore = groupScores.WinnerNodeScore
	nextPassView = winnerView
	return
}

func (m *multiSimulator) processSimulationGroupResults(scorer service.NodeScorer, groupResult *service.SimulationGroupResult) (simGroupScores service.SimulationGroupScores, winningView minkapi.View, err error) {
	var (
		nodeScores []service.NodeScore
		nodeScore  service.NodeScore
	)
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err = scorer.Compute(mapSimulationResultToNodeScoreArgs(sr))
		if err != nil {
			err = fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", service.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
			return
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerNodeScore, err := scorer.Select(nodeScores)
	if err != nil {
		err = fmt.Errorf("%w: node score selection failed for group %q: %w", service.ErrSelectNodeScore, groupResult.Name, err)
		return
	}
	simGroupScores = service.SimulationGroupScores{
		AllNodeScores:   nodeScores,
		WinnerNodeScore: winnerNodeScore,
	}
	if winnerNodeScore == nil {
		return
	}
	for _, sr := range groupResult.SimulationResults {
		if sr.Name == winnerNodeScore.Name {
			winningView = sr.View
			break
		}
	}
	if winningView == nil {
		err = fmt.Errorf("%w: winning view not found for winning node score %q of group %q", service.ErrSelectNodeScore, winnerNodeScore.Name, groupResult.Name)
		return
	}
	return
}

func mapSimulationResultToNodeScoreArgs(simResult service.SimulationResult) service.NodeScorerArgs {
	return service.NodeScorerArgs{
		ID:                      simResult.Name,
		ScaledNodePlacement:     simResult.ScaledNodePlacements[0],
		ScaledNodePodAssignment: &simResult.ScaledNodePodAssignments[0],
		OtherNodePodAssignments: simResult.OtherNodePodAssignments,
		LeftOverUnscheduledPods: simResult.LeftoverUnscheduledPods,
	}
}
