// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/planner/util"

	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScaleOutSimulator = (*SimulatorSingleNodeMultiSim)(nil)
)

// SimulatorSingleNodeMultiSim is a Simulator that implements ScaleOutSimulator for the SimulatorStrategySingleNodeMultiSim.
type SimulatorSingleNodeMultiSim struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	nodeScorer        plannerapi.NodeScorer
	traceDir          string
	state             simulatorState
	simulatorConfig   plannerapi.SimulatorConfig
}

type simulatorState struct {
	requestView          minkapi.View
	simulationFactory    plannerapi.SimulationFactory
	request              *plannerapi.Request
	planResultCh         chan plannerapi.ScaleOutPlanResult
	simulationViews      []minkapi.View
	simulationGroups     []plannerapi.ScaleOutSimGroup
	mu                   sync.Mutex
	simulationRunCounter atomic.Uint32
}

// New creates a new plannerapi.ScaleOutSimulator that runs multiple simulations concurrently.
// This is a factory function that supports type plannerapi.ScaleOutSimulatorFactory.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	return &SimulatorSingleNodeMultiSim{
		simulatorConfig:   args.Config,
		viewAccess:        args.ViewAccess,
		schedulerLauncher: args.SchedulerLauncher,
		storageMetaAccess: args.StorageMetaAccess,
		nodeScorer:        args.NodeScorer,
		traceDir:          args.TraceDir,
	}, nil
}

// Simulate constructs multiple ScaleOutSimulation's, groups them into ScaleOutSimGroup's, orders the groups and runs
// passes where each group is taken in order and simulations run concurrently to get ScaleOutSimResult's. The NodeScorer
// is used to get ScaleOutSimGroupPassScores; the winner NodeScore determines the simulation View for the next pass,
// until there are no more passes possible for the ScaleOutSimGroup. Then the ScaleOutSimGroupCycleResult is produced.
// If the ScalingAdviceGenerationMode is Incremental, a ScaleOutPlanResult is produced from this one cycle result and
// sent on the planResultCh, otherwise the cycle result is stored until all cycles are finished. Following which, a
// cumulative ScaleOutPlanResult is determined from all ScaleOutSimGroupCycleResult's obtained so far and sent on the planResultCh.
func (m *SimulatorSingleNodeMultiSim) Simulate(ctx context.Context, request *plannerapi.Request, simulationCreator plannerapi.SimulationFactory) <-chan plannerapi.ScaleOutPlanResult {
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
	return m.state.planResultCh
}

func (m *SimulatorSingleNodeMultiSim) doSimulate(ctx context.Context) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	m.state.requestView, err = m.viewAccess.GetSandboxViewOverDelegate(ctx, "request-"+m.state.request.ID, m.viewAccess.GetBaseView())
	if err != nil {
		return
	}

	if err = util.PopulateView(ctx, m.state.requestView, &m.state.request.Snapshot); err != nil {
		err = fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
		return
	}

	// Run static PVC<->PV Binding for Immediate VolumeBinding mode. Can be done just once for in the requestView for all simulations
	if _, err = util.BindClaimsAndVolumesForImmediateMode(ctx, m.state.requestView); err != nil {
		return
	}

	err = viewutil.LogDumpObjects(ctx, "requestView", m.state.requestView)
	if err != nil {
		log.Info("failed to dump view objects", "view", m.state.requestView.GetName(), "error", err)
	}

	m.state.simulationGroups, err = m.createAndGroupSimulation()
	if err != nil {
		return
	}

	err = m.runStabilizationCyclesForAllGroups(ctx)
	return
}

// Close closes all the resources of this simulator's state: all simulation minkapi views, resets simulation run counters,
// clears any ScaleOutSimGroup's, clears the planner Request, etc.
func (m *SimulatorSingleNodeMultiSim) Close() error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
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

func (m *SimulatorSingleNodeMultiSim) createAndGroupSimulation() ([]plannerapi.ScaleOutSimGroup, error) {
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
	return groupSimulations(m.state.request.GetRef(), allSimulations)
}

// runStabilizationCyclesForAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled
// pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running stabilization cycles for each group it will obtain
// the winning node scores and leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the
// simulator's result channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and
// leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the simulator's result channel.
func (m *SimulatorSingleNodeMultiSim) runStabilizationCyclesForAllGroups(ctx context.Context) (err error) {
	var (
		allWinnerNodeScores     []plannerapi.NodeScore
		simGroupCycleResult     plannerapi.ScaleOutSimGroupCycleResult
		allSimGroupCycleResults []plannerapi.ScaleOutSimGroupCycleResult
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
			log.V(2).Info("No winning node scores produced for group. Continuing to next group.")
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
		allSimGroupCycleResults = append(allSimGroupCycleResults, simGroupCycleResult)
		if len(simGroupCycleResult.LeftoverUnscheduledPods) == 0 {
			log.V(2).Info("Ending further runStabilizationCycleForGroup since there are no LeftoverUnscheduledPods.")
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
func (m *SimulatorSingleNodeMultiSim) runStabilizationCycleForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (cycleResult plannerapi.ScaleOutSimGroupCycleResult, err error) {
	var (
		winningNodeScore *plannerapi.NodeScore
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
				log.V(2).Info("No winning node score produced in pass. Ending group passes.")
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
	}
}

// runSinglePassForGroup runs all simulations in the given simulation group once over the provided passView, obtains the SimulationGroupRunResult,
// invokes the NodeScorer for each valid ScaleOutSimResult to compute the NodeScore and aggregates scores into the ScaleOutSimGroupPassScores - which includes the WinnerScore if any.
// If there is a WinnerScore among the SimulationRunResults, within the SimulationGroupRunResult, it is returned along with the nextGroupView.
// If there is no WinnerScore then return nil for both winnerNodeScore and the nextPassView.
func (m *SimulatorSingleNodeMultiSim) runSinglePassForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (nextGroupPassView minkapi.View, winnerNodeScore *plannerapi.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupScores plannerapi.ScaleOutSimGroupPassScores
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
		log.V(2).Info("simulation group did not produce any WinnerScore for this pass.")
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

func (m *SimulatorSingleNodeMultiSim) createSandboxView(ctx context.Context, name string, groupPassView minkapi.View) (minkapi.View, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	sandboxView, err := m.viewAccess.GetSandboxViewOverDelegate(ctx, name, groupPassView)
	if err != nil {
		return nil, err
	}
	m.state.simulationViews = append(m.state.simulationViews, sandboxView)
	return sandboxView, nil
}

func (m *SimulatorSingleNodeMultiSim) processSimulationGroupRunResults(log logr.Logger, simulationGroupName string, simulationRunResults []plannerapi.ScaleOutSimResult) (simGroupRunScores plannerapi.ScaleOutSimGroupPassScores, winningView minkapi.View, err error) {
	var nodeScore plannerapi.NodeScore

	for _, sr := range simulationRunResults {
		if len(sr.ScaledNodePodAssignments) == 0 {
			log.V(2).Info("No ScaledNodePodAssignments for simulation, skipping NodeScoring", "simulationName", sr.Name, "simulatedNodePlacement", sr.ScaledNodePlacements[0])
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

func mapSimulationResultToNodeScoreArgs(simResult plannerapi.ScaleOutSimResult) plannerapi.NodeScorerArgs {
	return plannerapi.NodeScorerArgs{
		ID:                      simResult.Name,
		ScaledNodePlacement:     simResult.ScaledNodePlacements[0],
		ScaledNodePodAssignment: &simResult.ScaledNodePodAssignments[0],
		OtherNodePodAssignments: simResult.OtherNodePodAssignments,
		LeftOverUnscheduledPods: simResult.LeftoverUnscheduledPods,
	}
}
