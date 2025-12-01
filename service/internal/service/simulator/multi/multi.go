package multi

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/service/internal/service/simulator"
	"github.com/gardener/scaling-advisor/service/scorer"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

var _ service.Simulator = (*multiSimulator)(nil)

// TODO find a better word for multiSimulator.
type multiSimulator struct {
	viewAccess           minkapi.ViewAccess
	resourceWeigher      service.ResourceWeigher
	pricingAccess        service.InstancePricingAccess
	nodeScorer           service.NodeScorer
	schedulerLauncher    service.SchedulerLauncher
	simulationCreator    service.SimulationCreator
	request              *service.ScalingAdviceRequest
	simulationRunCounter atomic.Uint32
}

// NewSimulator creates a new service.Simulator that runs multiple simulations concurrently.
func NewSimulator(viewAccess minkapi.ViewAccess, resourceWeigher service.ResourceWeigher, pricingAccess service.InstancePricingAccess, schedulerLauncher service.SchedulerLauncher, req *service.ScalingAdviceRequest) (service.Simulator, error) {
	nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, pricingAccess, resourceWeigher)
	if err != nil {
		return nil, err
	}
	return &multiSimulator{
		viewAccess:        viewAccess,
		resourceWeigher:   resourceWeigher,
		pricingAccess:     pricingAccess,
		nodeScorer:        nodeScorer,
		schedulerLauncher: schedulerLauncher,
		simulationCreator: service.SimulationCreatorFunc(NewSimulation),
		request:           req,
	}, nil
}

func (m *multiSimulator) SimulateScaleOut(ctx context.Context, planConsumerFn service.ScaleOutPlanConsumeFunc) error {
	baseView := m.viewAccess.GetBaseView()
	err := simulator.SynchronizeBaseView(ctx, baseView, m.request.Snapshot)
	if err != nil {
		return err
	}

	m.simulationRunCounter.Store(0) // initialize it to 0.
	simulationGroups, err := m.createSimulationGroups(ctx, m.request)
	if err != nil {
		return err
	}
	return m.runAllGroups(ctx, baseView, simulationGroups, planConsumerFn)
}

func (m *multiSimulator) createSimulationGroups(ctx context.Context, request *service.ScalingAdviceRequest) ([]service.SimulationGroup, error) {
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
		TrackPollInterval: 10 * time.Millisecond,
	}
	return NewSimulation(simulationName, simArgs)
}

//func (m *multiSimulator) runPasses(ctx context.Context, groups []service.SimulationGroup, consumeFn minkapi.View) error {
//	ctxLog := logr.FromContextOrDiscard(ctx)
//	var (
//		allWinnerNodeScores     []service.NodeScore
//		leftoverUnscheduledPods []types.NamespacedName
//	)
//runPassesLoop:
//	for {
//		groupRunPassNum := m.simulationRunCounter.Load()
//		log := ctxLog.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
//		select {
//		case <-ctx.Done():
//			err := ctx.Err()
//			log.Error(err, "Simulator context done. Aborting pass runPassesLoop")
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
//				if err = createAndInvokePlanConsumer(log, m.request, passWinnerNodeScores, passLeftoverUnscheduledPods, consumeFn); err != nil {
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
//		return service.ErrNoScalingAdvice
//	}
//
//	if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
//		if err := createAndInvokePlanConsumer(ctxLog, m.request, allWinnerNodeScores, leftoverUnscheduledPods, consumeFn); err != nil {
//			return err
//		}
//	}
//	return nil
//}

// runPass iterates through the simulation groups. For each group, it runs the group specific simulation(s) and
// obtains the service.SimulationGroupResult which encapsulates the results of all simulations in the group.
// It then runs the NodeScorer against the service.SimulationGroupResult and selects a winner amongst all the computed
// service.NodeScore's. If there is no winner node score it will continue to the next service.SimulationGroup.
// If there is a winner node score, it appends to the returned slice of allWinnerNodeScores.
// If there are no leftover unscheduled pods after processing a group, it breaks the loop and returns.
//func (m *multiSimulator) runPass(ctx context.Context, groups []service.SimulationGroup) (allWinnerNodeScores []service.NodeScore, unscheduledPods []types.NamespacedName, err error) {
//	log := logr.FromContextOrDiscard(ctx)
//	var (
//		groupResult service.SimulationGroupResult
//		groupScores service.SimulationGroupScores
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
// If the request AdviceGenerationMode is Incremental, after running passes for each group it will obtain the winning node scores and leftover unscheduled pods to construct a scale-out plan and invokes the planConsumeFn.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and leftover unscheduled pods to construct a scale-out plan and invokes the planConsumeFn.
func (m *multiSimulator) runAllGroups(ctx context.Context, baseView minkapi.View, simGroups []service.SimulationGroup, planConsumeFn service.ScaleOutPlanConsumeFunc) (err error) {
	var (
		groupView               = baseView
		groupWinnerNodeScores   []service.NodeScore
		allWinnerNodeScores     []service.NodeScore
		leftoverUnscheduledPods []types.NamespacedName
		log                     = logr.FromContextOrDiscard(ctx)
	)
	for groupIndex := 0; groupIndex < len(simGroups); {
		group := simGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		groupView, groupWinnerNodeScores, leftoverUnscheduledPods, err = m.runAllPassesForGroup(grpCtx, groupView, group)
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
			if err = createAndInvokePlanConsumer(log, m.request, groupWinnerNodeScores, leftoverUnscheduledPods, planConsumeFn); err != nil {
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
		err = createAndInvokePlanConsumer(log, m.request, allWinnerNodeScores, leftoverUnscheduledPods, planConsumeFn)
	}
	return
}

// runAllPassesForGroup runs all passes for the given simulation group until there is no winner or there are no leftover unscheduled pods or the context is done.
func (m *multiSimulator) runAllPassesForGroup(ctx context.Context, groupView minkapi.View, group service.SimulationGroup) (nextPassView minkapi.View, groupWinnerNodeScores []service.NodeScore, leftoverUnscheduledPods []types.NamespacedName, err error) {
	var (
		groupRunPass     int
		winningNodeScore *service.NodeScore
	)
	nextPassView = groupView
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			log := logr.FromContextOrDiscard(ctx).WithValues("groupRunPass", groupRunPass)
			passCtx := logr.NewContext(ctx, log)
			nextPassView, winningNodeScore, err = m.runSinglePassForGroup(passCtx, nextPassView, group)
			if err != nil {
				return
			}
			// winningNodeScore being nil indicates that there are no more winning node score, further passes can be aborted.
			if winningNodeScore == nil {
				log.Info("No winning node score produced in pass. Ending group passes.")
				return
			}
			groupWinnerNodeScores = append(groupWinnerNodeScores, *winningNodeScore)
			// It captures the leftover unscheduled pods from the last winning node score.
			// If there is no winning node score in the current pass, the leftover unscheduled pods from the
			// previous pass will be retained.
			leftoverUnscheduledPods = winningNodeScore.UnscheduledPods
			if len(leftoverUnscheduledPods) == 0 {
				log.Info("All pods have been scheduled in pass")
				return
			}
		}
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView.
// If there is a winnerNodeScore among the simulations in the group, it is returned along with the nextPassView.
// If there is no winner then winner node score is nil and the nextPassView is nil.
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

func createAndInvokePlanConsumer(log logr.Logger, req *service.ScalingAdviceRequest, winnerNodeScores []service.NodeScore, leftoverUnscheduledPods []types.NamespacedName, consumeFn service.ScaleOutPlanConsumeFunc) error {
	log.Info("Invoking scale-out plan consume func")
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	scaleOutPlan := simulator.CreateScaleOutPlan(winnerNodeScores, existingNodeCountByPlacement, leftoverUnscheduledPods)
	err = consumeFn(scaleOutPlan)
	if err != nil {
		return fmt.Errorf("failed to invoke scale-out plan consumer: %w", err)
	}
	return nil
}
