// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package multinode provides implementation and helper routines of a ScaleOutSimulator that performs simulations that scale
// multiple nodes at a time
package multinode

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/planner/simulator/scaleout"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScaleOutSimulator = (*simulatorSingleSim)(nil)
)

// simulatorSingleSim is a Simulator that implements ScaleOutSimulator for the SimulatorStrategyMultiNodeSingleSim.
type simulatorSingleSim struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	state             *scaleout.SimulatorState
	traceDir          string
	simulatorConfig   plannerapi.SimulatorConfig
}

// New creates a new plannerapi.ScaleOutSimulator that runs simulations sequentially scaling multiple nodes from
// different NodeTemplates at the same priority.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	return &simulatorSingleSim{
		simulatorConfig:   args.Config,
		viewAccess:        args.ViewAccess,
		schedulerLauncher: args.SchedulerLauncher,
		storageMetaAccess: args.StorageMetaAccess,
		traceDir:          args.TraceDir,
	}, nil
}

func (s *simulatorSingleSim) Close() error {
	return s.state.Reset()
}

func (s *simulatorSingleSim) Simulate(ctx context.Context, request *plannerapi.Request, simulationFactory plannerapi.SimulationFactory) (planResult <-chan plannerapi.ScaleOutPlanResult) {
	s.state = scaleout.NewSimulatorState(request, s.simulatorConfig, simulationFactory, s.viewAccess)
	go func() {
		defer close(s.state.ResultCh)
		if err := s.doSimulate(ctx); err != nil {
			scaleout.SendPlanError(s.state.ResultCh, request.GetRef(), err)
		}
	}()
	return s.state.ResultCh
}

func (s *simulatorSingleSim) doSimulate(ctx context.Context) (err error) {
	if err = s.state.InitializeView(ctx); err != nil {
		return
	}
	s.state.SimulationGroups, err = s.createAndGroupSimulations(ctx)
	if err != nil {
		return
	}
	err = s.runAllGroups(ctx)
	return
}

func (s *simulatorSingleSim) runAllGroups(ctx context.Context) (err error) {
	var (
		log           = logr.FromContextOrDiscard(ctx)
		groupPassView = s.state.RequestView()
		simResults    []plannerapi.ScaleOutSimResult
		allSimResults []plannerapi.ScaleOutSimResult
	)
	for groupIndex, group := range s.state.SimulationGroups {
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name()) // in-loop log enhanced with further params
		passCtx := logr.NewContext(ctx, log)
		if simResults, groupPassView, err = s.runPassForGroup(passCtx, group, groupPassView); err != nil {
			return
		}
		if len(simResults) > 0 {
			allSimResults = append(allSimResults, simResults...)
		}
	}
	if s.state.Request.AdviceGenerationMode.IsAllAtOnce() {
		err = scaleout.SendPlanResultUsingSimResults(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(), allSimResults)
	}
	return
}

func (s *simulatorSingleSim) runPassForGroup(ctx context.Context, group plannerapi.ScaleOutSimGroup, groupPassView minkapi.View) (simResults []plannerapi.ScaleOutSimResult, nextGroupPassView minkapi.View, err error) {
	//var (
	//	log = logr.FromContextOrDiscard(ctx)
	//)
	simResults, err = group.Run(ctx, func(ctx context.Context, name string) (minkapi.View, error) {
		return s.state.CreateSandboxView(ctx, name, groupPassView)
	})
	if err != nil {
		return
	}
	if len(simResults) == 0 {
		nextGroupPassView = groupPassView
		return
	} else {
		nextGroupPassView = simResults[0].View // all simResults share the same View in this strategy
	}
	if s.state.Request.AdviceGenerationMode.IsIncremental() {
		err = scaleout.SendPlanResultUsingSimResults(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(), simResults)
	}
	return
}

func (s *simulatorSingleSim) createAndGroupSimulations(ctx context.Context) ([]plannerapi.ScaleOutSimGroup, error) {
	var (
		allScaleOutNodeTemplates = scaleout.CreateAllNodeTemplates(s.state.Request.Constraint.Spec.NodePools)
		templatesByPriority      = scaleout.GroupScaleOutNodeTemplatesByPriority(allScaleOutNodeTemplates)
		allSimulations           = make([]plannerapi.ScaleOutSimulation, 0, len(templatesByPriority))
		log                      = logr.FromContextOrDiscard(ctx)
		simNum                   = 0
	)
	for pk, templates := range templatesByPriority {
		simulationName := fmt.Sprintf("sim-%d_%s", simNum, pk.String())
		simArgs := plannerapi.ScaleOutSimArgs{
			Name:              simulationName,
			RunCounter:        s.state.SimRunCounter,
			SchedulerLauncher: s.schedulerLauncher,
			StorageMetaAccess: s.storageMetaAccess,
			Config:            s.simulatorConfig,
			NodeTemplates:     templates,
			Strategy:          commontypes.SimulatorStrategyMultiNodeSingleSim,
		}
		sim, err := s.state.SimulationFactory.NewScaleOut(simArgs)
		if err != nil {
			return nil, err
		}
		log.V(3).Info("created simulation", "simulationName", simulationName)
		allSimulations = append(allSimulations, sim)
		simNum++
	}
	return scaleout.CreateScaleOutSimGroups(s.state.Request.GetRef(), allSimulations)
}
