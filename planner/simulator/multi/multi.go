// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/planner/util"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
)

var _ plannerapi.ScaleOutSimulator = (*multiSimulator)(nil)

// TODO find a better word for multiSimulator.
type multiSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	nodeScorer        plannerapi.NodeScorer
	// simulationCreator is a factory interface to create Simulations. This allows testing either the ScalingPlanner or MultiSimulator
	// with mock simulations.
	simulationCreator    plannerapi.SimulationCreator
	request              *plannerapi.ScalingAdviceRequest
	simulatorConfig      plannerapi.SimulatorConfig
	simulationRunCounter atomic.Uint32
}

// NewScaleOutSimulator creates a new plannerapi.ScaleOutSimulator that runs multiple simulations concurrently.
func NewScaleOutSimulator(viewAccess minkapi.ViewAccess, schedulerLauncher plannerapi.SchedulerLauncher, nodeScorer plannerapi.NodeScorer, simulatorConfig plannerapi.SimulatorConfig, req *plannerapi.ScalingAdviceRequest) (plannerapi.ScaleOutSimulator, error) {
	return &multiSimulator{
		viewAccess:        viewAccess,
		schedulerLauncher: schedulerLauncher,
		nodeScorer:        nodeScorer,
		simulatorConfig:   simulatorConfig,
		simulationCreator: plannerapi.SimulationCreatorFunc(NewSimulation),
		request:           req,
	}, nil
}

func (m *multiSimulator) Simulate(ctx context.Context, resultCh chan<- plannerapi.ScalingPlanResult) {
	var err error
	defer func() {
		if err != nil {
			util.SendPlanError(resultCh, m.request.GetRef(), err)
		}
	}()
	baseView := m.viewAccess.GetBaseView()
	if err = util.SynchronizeBaseView(ctx, baseView, m.request.Snapshot); err != nil {
		return
	}

	m.simulationRunCounter.Store(0) // initialize it to 0.
	simulationGroups, err := m.createSimulationGroups(m.request)
	if err != nil {
		return
	}
	err = m.runCyclesForAllGroups(ctx, baseView, simulationGroups, resultCh)
}

func (m *multiSimulator) createSimulationGroups(request *plannerapi.ScalingAdviceRequest) ([]plannerapi.SimulationGroup, error) {
	var allSimulations []plannerapi.Simulation
	simCount := 0
	for _, nodePool := range request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				var (
					sim plannerapi.Simulation
					err error
				)
				simCount++
				simulationName := fmt.Sprintf("sim-%d_%s_%s_%s", simCount, nodePool.Name, nodeTemplate.Name, zone)
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

func (m *multiSimulator) createSimulation(simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string) (plannerapi.Simulation, error) {
	simArgs := &plannerapi.SimulationArgs{
		RunCounter:        &m.simulationRunCounter,
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: m.schedulerLauncher,
		Config:            m.simulatorConfig,
	}
	return m.simulationCreator.Create(simulationName, simArgs)
}

// runCyclesForAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running stabilization cycles for each group it will obtain the winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and leftover unscheduled pods to construct a scale-out plan and sends it over the ScalingPlanResult channel.
func (m *multiSimulator) runCyclesForAllGroups(ctx context.Context, baseView minkapi.View, simGroups []plannerapi.SimulationGroup, resultCh chan<- plannerapi.ScalingPlanResult) (err error) {
	var (
		allWinnerNodeScores     []plannerapi.NodeScore
		simGroupCycleResult     plannerapi.SimulationGroupCycleResult
		allSimGroupCycleResults []plannerapi.SimulationGroupCycleResult
		log                     = logr.FromContextOrDiscard(ctx)
	)
	simGroupCycleResult.NextGroupView = baseView
	for groupIndex := 0; groupIndex < len(simGroups); {
		group := simGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		log.Info("Invoking runStabilizationCycleForGroup")
		simGroupCycleResult, err = m.runStabilizationCycleForGroup(grpCtx, simGroupCycleResult.NextGroupView, group)
		if err != nil {
			err = fmt.Errorf("failed to run all passes for group %q: %w", group.Name(), err)
			return
		}
		if len(simGroupCycleResult.WinnerNodeScores) == 0 {
			log.Info("No winning node scores produced for group. Continuing to next group.")
			groupIndex++
			continue
		}
		allWinnerNodeScores = append(allWinnerNodeScores, simGroupCycleResult.WinnerNodeScores...)
		if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeIncremental {
			log.Info("Sending ScalingPlanResult", "adviceGenerationMode", m.request.AdviceGenerationMode)
			m.simulationRunCounter.Load()
			if err = util.SendPlanResult(ctx, m.request, resultCh, m.simulationRunCounter.Load(), []plannerapi.SimulationGroupCycleResult{simGroupCycleResult}); err != nil {
				return
			}
		}
		allSimGroupCycleResults = append(allSimGroupCycleResults, simGroupCycleResult)
		if len(simGroupCycleResult.LeftoverUnscheduledPods) == 0 {
			log.Info("Ending further runStabilizationCycleForGroup since there are no LeftoverUnscheduledPods.")
			break
		}
	}
	if len(allWinnerNodeScores) == 0 {
		log.Info("No winning node scores produced by any pass of all simulation groups.")
		err = plannerapi.ErrNoScalingAdvice
		return
	}
	if m.request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
		log.Info("Sending ScalingPlanResult", "adviceGenerationMode", m.request.AdviceGenerationMode)
		err = util.SendPlanResult(ctx, m.request, resultCh, m.simulationRunCounter.Load(), allSimGroupCycleResults)
	}
	return
}

// runStabilizationCycleForGroup runs passes for the given simulation group until
//   - there are no leftover unscheduled pods after running a pass
//   - the simulation group has stabilized with no scheduled pods for all its child simulations.
//   - there is no winner node score after running a pass for the group
//   - the context is done.
func (m *multiSimulator) runStabilizationCycleForGroup(ctx context.Context, groupView minkapi.View, group plannerapi.SimulationGroup) (sgcr plannerapi.SimulationGroupCycleResult, err error) {
	var (
		winningNodeScore *plannerapi.NodeScore
	)
	sgcr.NextGroupView = groupView
	sgcr.NumPasses = 0
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			log := logr.FromContextOrDiscard(ctx).WithValues("groupRunNumPasses", sgcr.NumPasses)
			passCtx := logr.NewContext(ctx, log)
			sgcr.NumPasses++
			sgcr.NextGroupView, winningNodeScore, err = m.runSinglePassForGroup(passCtx, sgcr.NextGroupView, group)
			if err != nil {
				return
			}
			// winningNodeScore being nil indicates that there are no more winning node score, further passes can be aborted.
			if winningNodeScore == nil {
				log.Info("No winning node score produced in pass. Ending group passes.")
				return
			}
			if logutil.VerbosityFromContext(passCtx) >= 2 {
				err = viewutil.LogNodeAndPodNames(passCtx, sgcr.NextGroupView)
				if err != nil {
					return
				}
			}
			sgcr.WinnerNodeScores = append(sgcr.WinnerNodeScores, *winningNodeScore)
			// It captures the leftover unscheduled pods from the last winning node score.
			// If there is no winning node score in the current pass, the leftover unscheduled pods from the
			// previous pass will be retained.
			sgcr.LeftoverUnscheduledPods = winningNodeScore.UnscheduledPods
			if len(sgcr.LeftoverUnscheduledPods) == 0 {
				log.Info("All pods have been scheduled in pass")
				return
			}
		}
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView, obtains the SimulationGroupRunResult,
// , invokes the NodeScorer for each valid SimulationRunResult to compute the NodeScore and aggregates scores into the SimulationGroupRunScores - which includes the WinnerScore if any.
// If there is a WinnerScore among the SimulationRunResults within the SimulationGroupRunResult, it is returned along with the nextGroupView.
// If there is no WinnerScore then return nil for both winnerNodeScore and the nextPassView.
func (m *multiSimulator) runSinglePassForGroup(ctx context.Context, passView minkapi.View, group plannerapi.SimulationGroup) (nextPassView minkapi.View, winnerNodeScore *plannerapi.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupRunResult plannerapi.SimulationGroupRunResult
		groupScores    plannerapi.SimulationGroupRunScores
		winnerView     minkapi.View
	)
	getSimViewFn := func(ctx context.Context, name string) (minkapi.View, error) {
		return m.viewAccess.GetSandboxViewOverDelegate(ctx, name, passView)
	}
	groupRunResult, err = group.Run(ctx, getSimViewFn)
	if err != nil {
		return
	}
	groupScores, winnerView, err = m.processSimulationGroupRunResults(log, m.nodeScorer, &groupRunResult)
	if err != nil {
		return
	}
	if groupScores.WinnerScore == nil {
		log.Info("simulation group did not produce any WinnerScore for this pass.")
		nextPassView = passView
		return
	}
	winnerNodeScore = groupScores.WinnerScore
	nextPassView = winnerView
	nextPassView.GetEventSink().Reset()
	group.Reset()
	return
}

func (m *multiSimulator) processSimulationGroupRunResults(log logr.Logger, scorer plannerapi.NodeScorer, groupResult *plannerapi.SimulationGroupRunResult) (simGroupRunScores plannerapi.SimulationGroupRunScores, winningView minkapi.View, err error) {
	var (
		nodeScore plannerapi.NodeScore
	)
	for _, sr := range groupResult.SimulationResults {
		if len(sr.ScaledNodePodAssignments) == 0 {
			log.Info("No ScaledNodePodAssignments for simulation, skipping NodeScoring", "simulationName", sr.Name, "simulatedNodePlacement", sr.ScaledNodePlacements[0])
			continue
		}
		nodeScore, err = scorer.Compute(mapSimulationResultToNodeScoreArgs(sr))
		if err != nil {
			err = fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", plannerapi.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
			return
		}
		simGroupRunScores.AllScores = append(simGroupRunScores.AllScores, nodeScore)
	}
	if len(simGroupRunScores.AllScores) > 0 {
		simGroupRunScores.WinnerScore, err = scorer.Select(simGroupRunScores.AllScores)
		if err != nil {
			err = fmt.Errorf("%w: node score selection failed for group %q: %w", plannerapi.ErrSelectNodeScore, groupResult.Name, err)
			return
		}
	}
	if simGroupRunScores.WinnerScore == nil {
		return
	}
	for _, sr := range groupResult.SimulationResults {
		if sr.Name == simGroupRunScores.WinnerScore.Name {
			winningView = sr.View
			break
		}
	}
	if winningView == nil {
		err = fmt.Errorf("%w: winning view not found for winning node score %q of group %q", plannerapi.ErrSelectNodeScore, simGroupRunScores.WinnerScore.Name, groupResult.Name)
		return
	}
	return
}

func mapSimulationResultToNodeScoreArgs(simResult plannerapi.SimulationRunResult) plannerapi.NodeScorerArgs {
	return plannerapi.NodeScorerArgs{
		ID:                      simResult.Name,
		ScaledNodePlacement:     simResult.ScaledNodePlacements[0],
		ScaledNodePodAssignment: &simResult.ScaledNodePodAssignments[0],
		OtherNodePodAssignments: simResult.OtherNodePodAssignments,
		LeftOverUnscheduledPods: simResult.LeftoverUnscheduledPods,
	}
}
