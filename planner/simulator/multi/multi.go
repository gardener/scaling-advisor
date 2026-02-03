// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/planner/util"

	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
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
	state             simulatorState
	simulatorConfig   plannerapi.SimulatorConfig
}

type simulatorState struct {
	requestView          minkapi.View
	simulationCreator    plannerapi.SimulationCreator
	request              *plannerapi.Request
	planResultCh         chan plannerapi.ScaleOutPlanResult
	simulationViews      []minkapi.View
	simulationGroups     []plannerapi.SimulationGroup
	simulationRunCounter atomic.Uint32
}

// NewScaleOutSimulator creates a new plannerapi.ScaleOutSimulator that runs multiple simulations concurrently.
func NewScaleOutSimulator(simulatorConfig plannerapi.SimulatorConfig, viewAccess minkapi.ViewAccess,
	schedulerLauncher plannerapi.SchedulerLauncher, nodeScorer plannerapi.NodeScorer) (plannerapi.ScaleOutSimulator, error) {
	return &multiSimulator{
		simulatorConfig:   simulatorConfig,
		viewAccess:        viewAccess,
		schedulerLauncher: schedulerLauncher,
		nodeScorer:        nodeScorer,
	}, nil
}

func (m *multiSimulator) Simulate(ctx context.Context, request *plannerapi.Request, simulationCreator plannerapi.SimulationCreator) <-chan plannerapi.ScaleOutPlanResult {
	m.state = simulatorState{
		request:              request,
		simulationCreator:    simulationCreator,
		simulationRunCounter: atomic.Uint32{},
		planResultCh:         make(chan plannerapi.ScaleOutPlanResult),
	}
	go func() {
		defer close(m.state.planResultCh)
		if err := m.doSimulate(ctx); err != nil {
			util.SendScaleOutPlanError(m.state.planResultCh, request.GetRef(), err)
		}
	}()
	return m.state.planResultCh
}

func (m *multiSimulator) doSimulate(ctx context.Context) (err error) {
	m.state.requestView, err = m.viewAccess.GetSandboxViewOverDelegate(ctx, "request-"+m.state.request.ID, m.viewAccess.GetBaseView())
	if err != nil {
		return
	}

	if err = util.SynchronizeView(ctx, m.state.requestView, &m.state.request.Snapshot); err != nil {
		return err
	}

	_ = viewutil.LogNodeAndPodNames(ctx, "requestView", m.state.requestView)

	m.state.simulationGroups, err = m.createAndGroupSimulation()
	if err != nil {
		return
	}

	err = m.runStabilizationCyclesForAllGroups(ctx)
	return
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
	m.state.simulationCreator = nil
	clear(m.state.simulationGroups)
	m.state.request = nil
	return errors.Join(errs...)
}

func (m *multiSimulator) createAndGroupSimulation() ([]plannerapi.SimulationGroup, error) {
	var allSimulations []plannerapi.Simulation
	simCount := 0
	for _, nodePool := range m.state.request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				var (
					sim plannerapi.Simulation
					err error
				)
				simCount++
				simulationName := fmt.Sprintf("sim-%d_%s_%s_%s", simCount, nodePool.Name, nodeTemplate.Name, zone)
				simArgs := &plannerapi.SimulationArgs{
					RunCounter:        &m.state.simulationRunCounter,
					AvailabilityZone:  zone,
					NodePool:          &nodePool,
					NodeTemplateName:  nodeTemplate.Name,
					SchedulerLauncher: m.schedulerLauncher,
					Config:            m.simulatorConfig,
				}
				sim, err = m.state.simulationCreator.Create(simulationName, simArgs)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return groupSimulations(m.state.request.GetRef(), allSimulations)
}

// runStabilizationCyclesForAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled
// pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running stabilization cycles for each group it will obtain
// the winning node scores and leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the
// simulator's result channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and
// leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the simulator's result channel.
func (m *multiSimulator) runStabilizationCyclesForAllGroups(ctx context.Context) (err error) {
	var (
		allWinnerNodeScores     []plannerapi.NodeScore
		simGroupCycleResult     plannerapi.SimulationGroupCycleResult
		allSimGroupCycleResults []plannerapi.SimulationGroupCycleResult
		log                     = logr.FromContextOrDiscard(ctx)
	)
	simGroupCycleResult.NextGroupPassView = m.state.requestView
	for groupIndex := 0; groupIndex < len(m.state.simulationGroups); {
		group := m.state.simulationGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		log.V(3).Info("Invoking runStabilizationCycleForGroup")
		simGroupCycleResult, err = m.runStabilizationCycleForGroup(grpCtx, simGroupCycleResult.NextGroupPassView, group)
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
		if m.state.request.AdviceGenerationMode.IsIncremental() {
			log.V(4).Info("Sending ScalingPlanResult", "adviceGenerationMode", m.state.request.AdviceGenerationMode)
			if err = util.SendScaleOutPlanResult(ctx, m.state.planResultCh, m.state.request, m.state.simulationRunCounter.Load(),
				[]plannerapi.SimulationGroupCycleResult{simGroupCycleResult}); err != nil {
				return
			}
		}
		allSimGroupCycleResults = append(allSimGroupCycleResults, simGroupCycleResult)
		if len(simGroupCycleResult.LeftoverUnscheduledPods) == 0 {
			log.V(3).Info("Ending further runStabilizationCycleForGroup since there are no LeftoverUnscheduledPods.")
			break
		}
	}
	if len(allWinnerNodeScores) == 0 {
		log.V(3).Info("No winning node scores produced by any pass of all simulation groups.")
		err = plannerapi.ErrNoScaleOutPlan
		return
	}
	if m.state.request.AdviceGenerationMode.IsAllAtOnce() {
		log.V(4).Info("Sending ScalingPlanResult", "adviceGenerationMode", m.state.request.AdviceGenerationMode)
		err = util.SendScaleOutPlanResult(ctx, m.state.planResultCh, m.state.request, m.state.simulationRunCounter.Load(), allSimGroupCycleResults)
	}
	return
}

// runStabilizationCycleForGroup runs passes for the given simulation group until
//   - there are no leftover unscheduled pods after running a pass
//   - the simulation group has stabilized with no scheduled pods for all its child simulations.
//   - there is no winner node score after running a pass for the group
//   - the context is done.
func (m *multiSimulator) runStabilizationCycleForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.SimulationGroup) (sgcr plannerapi.SimulationGroupCycleResult, err error) {
	var (
		winningNodeScore *plannerapi.NodeScore
	)
	sgcr.NextGroupPassView = groupPassView
	sgcr.PassNum = 0
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			sgcr.PassNum++
			log := logr.FromContextOrDiscard(ctx).WithValues("groupRunPassNum", sgcr.PassNum)
			passCtx := logr.NewContext(ctx, log)
			sgcr.NextGroupPassView, winningNodeScore, err = m.runSinglePassForGroup(passCtx, sgcr.NextGroupPassView, group)
			if err != nil {
				return
			}
			// winningNodeScore being nil indicates that there are no more winning node score, further passes can be aborted.
			if winningNodeScore == nil {
				log.Info("No winning node score produced in pass. Ending group passes.")
				return
			}
			if logutil.VerbosityFromContext(passCtx) > 3 {
				err = viewutil.LogNodeAndPodNames(passCtx, "post_runSinglePassForGroup", sgcr.NextGroupPassView)
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
				log.V(2).Info("All pods have been scheduled in pass")
				return
			}
		}
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView, obtains the SimulationGroupRunResult,
// invokes the NodeScorer for each valid SimulationRunResult to compute the NodeScore and aggregates scores into the SimulationGroupPassScores - which includes the WinnerScore if any.
// If there is a WinnerScore among the SimulationRunResults within the SimulationGroupRunResult, it is returned along with the nextGroupView.
// If there is no WinnerScore then return nil for both winnerNodeScore and the nextPassView.
func (m *multiSimulator) runSinglePassForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.SimulationGroup) (nextGroupPassView minkapi.View, winnerNodeScore *plannerapi.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupScores plannerapi.SimulationGroupPassScores
		winnerView  minkapi.View
	)
	simulationRunResults, err := group.Run(ctx, func(ctx context.Context, name string) (minkapi.View, error) {
		return m.createSandboxView(ctx, name, groupPassView)
	})
	if err != nil {
		return
	}
	groupScores, winnerView, err = m.processSimulationGroupRunResults(log, group.Name(), simulationRunResults)
	if err != nil {
		return
	}
	if groupScores.WinnerScore == nil {
		log.Info("simulation group did not produce any WinnerScore for this pass.")
		nextGroupPassView = groupPassView
		return
	}
	winnerNodeScore = groupScores.WinnerScore
	nextGroupPassView = winnerView
	err = ioutil.ResetAll(nextGroupPassView.GetEventSink(), group)
	if err != nil {
		err = fmt.Errorf("cannot reset event sink of view %q and/or simulation group %q: %w", nextGroupPassView.GetName(), group.Name(), err)
	}
	return
}

func (m *multiSimulator) createSandboxView(ctx context.Context, name string, groupPassView minkapi.View) (minkapi.View, error) {
	sandboxView, err := m.viewAccess.GetSandboxViewOverDelegate(ctx, name, groupPassView)
	if err != nil {
		return nil, err
	}
	m.state.simulationViews = append(m.state.simulationViews, sandboxView)
	return sandboxView, nil
}

func (m *multiSimulator) processSimulationGroupRunResults(log logr.Logger, simulationGroupName string, simulationRunResults []plannerapi.SimulationRunResult) (simGroupRunScores plannerapi.SimulationGroupPassScores, winningView minkapi.View, err error) {
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
	for _, sr := range simulationRunResults {
		if sr.Name == simGroupRunScores.WinnerScore.Name {
			winningView = sr.View
			break
		}
	}
	if winningView == nil {
		err = fmt.Errorf("%w: winning view not found for winning node score %q of group %q",
			plannerapi.ErrSelectNodeScore, simGroupRunScores.WinnerScore.Name, simulationGroupName)
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
