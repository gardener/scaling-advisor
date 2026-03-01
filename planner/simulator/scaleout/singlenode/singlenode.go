// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package snms provides implementation and helper routines of a ScaleOutSimulator that performs multiple simulations that scale a single
// node at a time.
package singlenode

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/gardener/scaling-advisor/planner/simulator/scaleout"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScaleOutSimulator = (*simulatorMultiSim)(nil)
)

// simulatorMultiSim is a Simulator that implements ScaleOutSimulator for the SimulatorStrategySingleNodeMultiSim.
type simulatorMultiSim struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	nodeScorer        plannerapi.NodeScorer
	traceDir          string
	state             scaleout.RequestState
	simulatorConfig   plannerapi.SimulatorConfig
}

// New creates a new plannerapi.ScaleOutSimulator that runs simulations for a single scaled node concurrently.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	return &simulatorMultiSim{
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
// If the ScalingAdviceGenerationMode is Incremental, a ScaleOutPlanResult is produced from this one-cycle result and
// sent on the planResultCh, otherwise the cycle result is stored until all cycles are finished. Following which, a
// cumulative ScaleOutPlanResult is determined from all ScaleOutSimGroupCycleResult's obtained so far and sent on the planResultCh.
func (s *simulatorMultiSim) Simulate(ctx context.Context, request *plannerapi.Request, simulationFactory plannerapi.SimulationFactory) <-chan plannerapi.ScaleOutPlanResult {
	s.state = scaleout.RequestStateWith(request, simulationFactory, s.viewAccess)
	go func() {
		defer close(s.state.ResultCh)
		if err := s.doSimulate(ctx); err != nil {
			scaleout.SendPlanError(s.state.ResultCh, request.GetRef(), err)
		}
	}()
	return s.state.ResultCh
}

func (s *simulatorMultiSim) doSimulate(ctx context.Context) (err error) {
	if err = s.state.Initialize(ctx); err != nil {
		return
	}
	s.state.SimulationGroups, err = s.createAndGroupSimulation()
	if err != nil {
		return
	}
	err = s.runStabilizationCyclesForAllGroups(ctx)
	return
}

// Close closes all the resources of this simulator's state: all simulation minkapi views, resets simulation run counters,
// clears any ScaleOutSimGroup's, clears the planner Request, etc.
func (s *simulatorMultiSim) Close() error {
	return s.state.Reset()
}

func (s *simulatorMultiSim) createAndGroupSimulation() ([]plannerapi.ScaleOutSimGroup, error) {
	var allSimulations []plannerapi.ScaleOutSimulation
	simCount := 0
	for _, nodePool := range s.state.Request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				var (
					sim plannerapi.ScaleOutSimulation
					err error
				)
				simCount++
				simulationName := fmt.Sprintf("sim-%d_%s_%s_%s", simCount, nodePool.Name, nodeTemplate.Name, zone)
				ptz := scaleout.CreateNodeArgs(nodePool, nodeTemplate, zone)
				simArgs := plannerapi.ScaleOutSimArgs{
					Name:              simulationName,
					RunCounter:        s.state.SimRunCounter,
					SchedulerLauncher: s.schedulerLauncher,
					StorageMetaAccess: s.storageMetaAccess,
					Config:            s.simulatorConfig,
					TraceDir:          s.traceDir,
					NodeTemplates:     []plannerapi.ScaleOutNodeTemplate{ptz},
					Strategy:          commontypes.SimulatorStrategySingleNodeMultiSim,
				}
				sim, err = s.state.SimulationFactory.NewScaleOut(simArgs)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return scaleout.GroupSimulations(s.state.Request.GetRef(), allSimulations)
}

// runStabilizationCyclesForAllGroups runs all simulation groups until there is no winner or there are no leftover unscheduled
// pods or the context is done.
// If the request AdviceGenerationMode is Incremental, after running stabilization cycles for each group, it will obtain
// the winning node scores and leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the
// simulator's result channel.
// If the request AdviceGenerationMode is AllAtOnce, after running all groups it will obtain all winning node scores and
// leftover unscheduled pods to construct a ScaleOutPlanResult and send it over the simulator's result channel.
func (s *simulatorMultiSim) runStabilizationCyclesForAllGroups(ctx context.Context) (err error) {
	var (
		allWinnerNodeScores     []plannerapi.NodeScore
		simGroupCycleResult     plannerapi.ScaleOutSimGroupCycleResult
		allSimGroupCycleResults []plannerapi.ScaleOutSimGroupCycleResult
		log                     = logr.FromContextOrDiscard(ctx)
	)
	simGroupCycleResult.NextGroupPassView = s.state.RequestView()
	for groupIndex := 0; groupIndex < len(s.state.SimulationGroups); {
		group := s.state.SimulationGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		log.V(3).Info("Invoking runStabilizationCycleForGroup")
		simGroupCycleResult, err = s.runStabilizationCycleForGroup(grpCtx, simGroupCycleResult.NextGroupPassView, group)
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
		if s.state.Request.AdviceGenerationMode.IsIncremental() {
			log.V(4).Info("Sending ScalingPlanResult", "adviceGenerationMode", s.state.Request.AdviceGenerationMode)
			if err = scaleout.SendPlanResult(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(),
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
	if s.state.Request.AdviceGenerationMode.IsAllAtOnce() {
		log.V(4).Info("Sending ScalingPlanResult", "adviceGenerationMode", s.state.Request.AdviceGenerationMode)
		err = scaleout.SendPlanResult(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(), allSimGroupCycleResults)
	}
	return
}

// runStabilizationCycleForGroup runs passes for the given simulation group until
//   - there are no leftover unscheduled pods after running a pass
//   - the simulation group has stabilized with no scheduled pods for all its child simulations.
//   - there is no winner node score after running a pass for the group
//   - the context is done.
func (s *simulatorMultiSim) runStabilizationCycleForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (cycleResult plannerapi.ScaleOutSimGroupCycleResult, err error) {
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
			cycleResult.NextGroupPassView, winningNodeScore, err = s.runSinglePassForGroup(passCtx, cycleResult.NextGroupPassView, group)
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
func (s *simulatorMultiSim) runSinglePassForGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (nextGroupPassView minkapi.View, winnerNodeScore *plannerapi.NodeScore, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupScores plannerapi.ScaleOutSimGroupPassScores
		winnerView  minkapi.View
	)
	simulationRunResults, err := group.Run(ctx, func(ctx context.Context, name string) (minkapi.View, error) {
		return s.state.CreateSandboxView(ctx, name, groupPassView)
	})
	if err != nil {
		return
	}
	groupScores, winnerView, err = s.processSimulationGroupRunResults(log, group.Name(), simulationRunResults)
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

func (s *simulatorMultiSim) processSimulationGroupRunResults(log logr.Logger, simulationGroupName string, simulationRunResults []plannerapi.ScaleOutSimResult) (simGroupRunScores plannerapi.ScaleOutSimGroupPassScores, winningView minkapi.View, err error) {
	var nodeScore plannerapi.NodeScore

	for _, sr := range simulationRunResults {
		if len(sr.NodePodAssignments) == 0 {
			log.V(2).Info("No NodePodAssignments for simulation, skipping NodeScoring", "simulationName", sr.Name)
			continue
		}
		nodeScore, err = s.nodeScorer.Compute(mapSimulationResultToNodeScoreArgs(sr))
		if err != nil {
			err = fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", plannerapi.ErrComputeNodeScore, sr.Name, simulationGroupName, err)
			return
		}
		simGroupRunScores.AllScores = append(simGroupRunScores.AllScores, nodeScore)
	}
	if len(simGroupRunScores.AllScores) > 0 {
		simGroupRunScores.WinnerScore, err = s.nodeScorer.Select(simGroupRunScores.AllScores)
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
	scaleOutNodePlacement := slices.Collect(maps.Keys(simResult.NodePlacements))[0]
	return plannerapi.NodeScorerArgs{
		ID:                      simResult.Name,
		ScaledNodePlacement:     scaleOutNodePlacement,
		ScaledNodePodAssignment: &simResult.NodePodAssignments[0],
		OtherNodePodAssignments: simResult.OtherNodePodAssignments,
		LeftOverUnscheduledPods: simResult.LeftoverUnscheduledPods,
	}
}
