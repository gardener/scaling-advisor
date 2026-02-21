// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/planner/util"
	"sync/atomic"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

var (
	_ plannerapi.ScaleOutSimulator = (*multiSimulator)(nil)
)

// TODO find a better word for multiSimulator.
type multiSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	nodeScorer        plannerapi.NodeScorer
	state             simulatorState
	simulatorConfig   plannerapi.SimulatorConfig
	traceDir          string
}

type simulatorState struct {
	requestView          minkapi.View
	simulationFactory    plannerapi.SimulationFactory
	request              *plannerapi.Request
	planResultCh         chan plannerapi.ScaleOutPlanResult
	simulationViews      []minkapi.View
	simulationGroups     []plannerapi.ScaleOutSimGroup
	simulationRunCounter atomic.Uint32
}

// NewScaleOutSimulator creates a new plannerapi.ScaleOutSimulator that runs multiple simulations concurrently.
// This is a factory function that supports type plannerapi.ScaleOutSimulatorFactory.
func NewScaleOutSimulator(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	return &multiSimulator{
		simulatorConfig:   args.Config,
		viewAccess:        args.ViewAccess,
		schedulerLauncher: args.SchedulerLauncher,
		storageMetaAccess: args.StorageMetaAccess,
		nodeScorer:        args.NodeScorer,
		traceDir:          args.TraceDir,
	}, nil
}

func (m *multiSimulator) Simulate(ctx context.Context, request *plannerapi.Request, simulationCreator plannerapi.SimulationFactory) <-chan plannerapi.ScaleOutPlanResult {
	m.state = simulatorState{
		request:              request,
		simulationFactory:    simulationCreator,
		simulationRunCounter: atomic.Uint32{},
		planResultCh:         make(chan plannerapi.ScaleOutPlanResult),
	}
	go func() {
		defer close(m.state.planResultCh)
		if err := m.doSimulate(ctx); err != nil {
			util.SendScaleOutPlanError(m.state.planResultCh, request.GetRef(), err)
		}
	}()
	baseView := m.viewAccess.GetBaseView()
	if err = simulator.SynchronizeBaseView(ctx, baseView, m.request.Snapshot); err != nil {
		return
	}

	if err = util.PopulateView(ctx, m.state.requestView, &m.state.request.Snapshot); err != nil {
		err = fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
		return
	}

	_ = viewutil.LogDumpObjects(ctx, "requestView", m.state.requestView)

	m.state.simulationGroups, err = m.createAndGroupSimulation()
	if err != nil {
		return
	}
	err = m.runAllGroups(ctx, baseView, simulationGroups, resultCh)
}

func (m *multiSimulator) Close() error {
	var errs []error
	for _, v := range m.state.simulationViews {
		if err := v.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	clear(m.state.simulationViews)
	m.state.simulationRunCounter.Store(0)
	m.state.simulationFactory = nil
	clear(m.state.simulationGroups)
	m.state.request = nil
	return errors.Join(errs...)
}

func (m *multiSimulator) createAndGroupSimulation() ([]plannerapi.ScaleOutSimGroup, error) {
	var allSimulations []plannerapi.ScaleOutSimulation
	simCount := 0
	for _, nodePool := range m.state.request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				var (
					sim plannerapi.ScaleOutSimulation
					err error
				)
				simCount++
				simulationName := fmt.Sprintf("sim-%d_%s_%s_%s", simCount, nodePool.Name, nodeTemplate.Name, zone)
				simArgs := plannerapi.ScaleOutSimArgs{
					RunCounter:        &m.state.simulationRunCounter,
					AvailabilityZone:  zone,
					NodePool:          &nodePool,
					NodeTemplateName:  nodeTemplate.Name,
					SchedulerLauncher: m.schedulerLauncher,
					StorageMetaAccess: m.storageMetaAccess,
					Config:            m.simulatorConfig,
					TraceDir:          m.traceDir,
				}
				sim, err = m.state.simulationFactory.NewScaleOut(simulationName, simArgs)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return createSimulationGroups(allSimulations)
}

func (m *multiSimulator) createSimulation(simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string) (planner.Simulation, error) {
	simArgs := &planner.SimulationArgs{
		RunCounter:        &m.simulationRunCounter,
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: m.schedulerLauncher,
		Config:            m.simulatorConfig,
	}
	return m.simulationCreator.Create(simulationName, simArgs)
}

// runAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running passes for each group it will obtain the winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
func (m *multiSimulator) runAllGroups(ctx context.Context, baseView minkapi.View, simGroups []planner.SimulationGroup, resultCh chan<- planner.ScalingPlanResult) (err error) {
	var (
		allWinnerNodeScores     []plannerapi.NodeScore
		simGroupCycleResult     plannerapi.ScaleOutSimGroupCycleResult
		allSimGroupCycleResults []plannerapi.ScaleOutSimGroupCycleResult
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
		if len(simGroupRunResult.WinnerNodeScores) == 0 {
			log.Info("No winning node scores produced for group. Continuing to next group.")
			groupIndex++
			continue
		}
		allWinnerNodeScores = append(allWinnerNodeScores, simGroupCycleResult.WinnerNodeScores...)
		if m.state.request.AdviceGenerationMode.IsIncremental() {
			log.V(4).Info("Sending ScalingPlanResult", "adviceGenerationMode", m.state.request.AdviceGenerationMode)
			if err = util.SendScaleOutPlanResult(ctx, m.state.planResultCh, m.state.request, m.state.simulationRunCounter.Load(),
				[]plannerapi.ScaleOutSimGroupCycleResult{simGroupCycleResult}); err != nil {
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
		err = planner.ErrNoScalingAdvice
		return
	}
	if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
		log.Info("Sending all-at-once scale-out plan")
		err = util.SendPlanResult(m.request, simGroupRunResult, resultCh)
	}
	return
}

// runStabilizationCycleForGroup runs passes for the given simulation group until
//   - there are no leftover unscheduled pods after running a pass
//   - the simulation group has stabilized with no scheduled pods for all its child simulations.
//   - there is no winner node score after running a pass for the group
//   - the context is done.
func (m *multiSimulator) runStabilizationCycleForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (cycleResult plannerapi.ScaleOutSimGroupCycleResult, err error) {
	var (
		winningNodeScore *planner.NodeScore
	)
	cycleResult.NextGroupPassView = groupPassView
	cycleResult.PassNum = 0
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			cycleResult.PassNum++
			log := logr.FromContextOrDiscard(ctx).WithValues("groupRunPassNum", cycleResult.PassNum)
			passCtx := logr.NewContext(ctx, log)
			cycleResult.NextGroupPassView, winningNodeScore, err = m.runSinglePassForGroup(passCtx, cycleResult.NextGroupPassView, group)
			if err != nil {
				return
			}
			// winningNodeScore being nil indicates that there are no more winning node score, further passes can be aborted.
			if winningNodeScore == nil {
				log.Info("No winning node score produced in pass. Ending group passes.")
				return
			}
			if logutil.VerbosityFromContext(passCtx) > 3 {
				err = viewutil.LogDumpObjects(passCtx, "post_runSinglePassForGroup", cycleResult.NextGroupPassView)
				if err != nil {
					return
				}
			}
			cycleResult.WinnerNodeScores = append(cycleResult.WinnerNodeScores, *winningNodeScore)
			// It captures the leftover unscheduled pods from the last winning node score.
			// If there is no winning node score in the current pass, the leftover unscheduled pods from the
			// previous pass will be retained.
			cycleResult.LeftoverUnscheduledPods = winningNodeScore.UnscheduledPods
			if len(cycleResult.LeftoverUnscheduledPods) == 0 {
				log.V(2).Info("All pods have been scheduled in pass")
				return
			}
		}
		sgrr.NumPasses++
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView, obtains the SimulationGroupRunResult,
// invokes the NodeScorer for each valid ScaleOutSimResult to compute the NodeScore and aggregates scores into the ScaleOutSimGroupPassScores - which includes the WinnerScore if any.
// If there is a WinnerScore among the SimulationRunResults within the SimulationGroupRunResult, it is returned along with the nextGroupView.
// If there is no WinnerScore then return nil for both winnerNodeScore and the nextPassView.
func (m *multiSimulator) runSinglePassForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (nextGroupPassView minkapi.View, winnerNodeScore *plannerapi.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupScores plannerapi.ScaleOutSimGroupPassScores
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

func (m *multiSimulator) processSimulationGroupResults(scorer planner.NodeScorer, groupResult *planner.SimulationGroupResult) (simGroupScores planner.SimulationGroupScores, winningView minkapi.View, err error) {
	var (
		nodeScores []planner.NodeScore
		nodeScore  planner.NodeScore
	)
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err = scorer.Compute(mapSimulationResultToNodeScoreArgs(sr))
		if err != nil {
			err = fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", planner.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
			return
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerNodeScore, err := scorer.Select(nodeScores)
	if err != nil {
		return nil, err
	}
	m.state.simulationViews = append(m.state.simulationViews, sandboxView)
	return sandboxView, nil
}

func (m *multiSimulator) processSimulationGroupRunResults(log logr.Logger, simulationGroupName string, simulationRunResults []plannerapi.ScaleOutSimResult) (simGroupRunScores plannerapi.ScaleOutSimGroupPassScores, winningView minkapi.View, err error) {
	var nodeScore plannerapi.NodeScore

	for _, sr := range simulationRunResults {
		if len(sr.ScaledNodePodAssignments) == 0 {
			log.Info("No ScaledNodePodAssignments for simulation, skipping NodeScoring", "simulationName", sr.Name, "simulatedNodePlacement", sr.ScaledNodePlacements[0])
			continue
		}
		nodeScore, err = m.nodeScorer.Compute(mapSimulationResultToNodeScoreArgs(sr))
		if err != nil {
			err = fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", plannerapi.ErrComputeNodeScore, sr.Name, simulationGroupName, err)
			return
		}
		simGroupRunScores.AllScores = append(simGroupRunScores.AllScores, nodeScore)
	}
	if len(simGroupRunScores.AllScores) > 0 {
		simGroupRunScores.WinnerScore, err = m.nodeScorer.Select(simGroupRunScores.AllScores)
		if err != nil {
			err = fmt.Errorf("%w: node score selection failed for group %q: %w", plannerapi.ErrSelectNodeScore, simulationGroupName, err)
			return
		}
	}
	if simGroupRunScores.WinnerScore == nil {
		return
	}
	simGroupScores = planner.SimulationGroupScores{
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
		err = fmt.Errorf("%w: winning view not found for winning node score %q of group %q", planner.ErrSelectNodeScore, winnerNodeScore.Name, groupResult.Name)
		return
	}
	return
}

func mapSimulationResultToNodeScoreArgs(simResult plannerapi.ScaleOutSimResult) plannerapi.NodeScorerArgs {
	return plannerapi.NodeScorerArgs{
		ID:                      simResult.Name,
		ScaledNodePlacement:     simResult.ScaledNodePlacements[0],
		ScaledNodePodAssignment: &simResult.ScaledNodePodAssignments[0],
		OtherNodePodAssignments: simResult.OtherNodePodAssignments,
		LeftOverUnscheduledPods: simResult.LeftoverUnscheduledPods,
	}
}
