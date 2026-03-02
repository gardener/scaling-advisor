// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package multinode provides implementation and helper routines of a ScaleOutSimulator that performs simulations that scale
// multiple nodes at a time.
package multinode

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/simulator/scaleout"
	"github.com/go-logr/logr"
	"maps"
	"slices"
)

var (
	_ plannerapi.ScaleOutSimulator = (*simulatorSingleSim)(nil)
)

// simulatorSingleSim is a Simulator that implements ScaleOutSimulator for the SimulatorStrategyMultiNodeSingleSim.
type simulatorSingleSim struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	traceDir          string
	state             scaleout.RequestState
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
	s.state = scaleout.RequestStateWith(request, s.simulatorConfig, simulationFactory, s.viewAccess)
	go func() {
		defer close(s.state.ResultCh)
		if err := s.doSimulate(ctx); err != nil {
			scaleout.SendPlanError(s.state.ResultCh, request.GetRef(), err)
		}
	}()
	return s.state.ResultCh
}

func (s *simulatorSingleSim) doSimulate(ctx context.Context) (err error) {
	if err = s.state.InitializeRequestView(ctx); err != nil {
		return
	}
	s.state.SimulationGroups, err = s.createAndGroupSimulations()
	if err != nil {
		return
	}
	err = s.runGroups(ctx)
	return
}

func (s *simulatorSingleSim) runGroups(ctx context.Context) error {
	var (
		log           = logr.FromContextOrDiscard(ctx)
		groupPassView = s.state.RequestView()
	)
	for groupIndex := 0; groupIndex < len(s.state.SimulationGroups); {
		group := s.state.SimulationGroups[groupIndex]
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name())
		grpCtx := logr.NewContext(ctx, log)
		log.V(3).Info("Invoking group.Run")
		s.runGroup(grpCtx, groupPassView, group)
	}
}

func (s *simulatorSingleSim) runGroup(ctx context.Context, groupPassView minkapi.View, group plannerapi.ScaleOutSimGroup) (nextGroupPassView minkapi.View, err error) {
	simResults, err := group.Run(ctx, func(ctx context.Context, name string) (minkapi.View, error) {
		return s.state.CreateSandboxView(ctx, name, groupPassView)
	})
	return
}

func (s *simulatorSingleSim) createAndGroupSimulations() ([]plannerapi.ScaleOutSimGroup, error) {
	var (
		allScaleOutNodeTemplates = scaleout.CreateAllNodeTemplates(s.state.Request.Constraint.Spec.NodePools)
		allSimulations           []plannerapi.ScaleOutSimulation
		templatesByPriority      = scaleout.GroupScaleOutNodeTemplatesByPriority(allScaleOutNodeTemplates)
	)
	priorityKeys := slices.Collect(maps.Keys(templatesByPriority))
	slices.SortFunc(priorityKeys, commontypes.CmpPriorityKeyDecreasing)
	for idx, key := range priorityKeys {
		templatesGroup := templatesByPriority[key]
		simulationName := fmt.Sprintf("sim-%d", idx)
		simArgs := plannerapi.ScaleOutSimArgs{
			Name:              simulationName,
			RunCounter:        s.state.SimRunCounter,
			SchedulerLauncher: s.schedulerLauncher,
			StorageMetaAccess: s.storageMetaAccess,
			Config:            s.simulatorConfig,
			TraceDir:          s.traceDir,
			NodeTemplates:     templatesGroup,
			Strategy:          commontypes.SimulatorStrategySingleNodeMultiSim,
		}
		sim, err := s.state.SimulationFactory.NewScaleOut(simArgs)
		if err != nil {
			return nil, err
		}
		allSimulations = append(allSimulations, sim)
	}
	return scaleout.CreateScaleOutSimGroups(s.state.Request.GetRef(), allSimulations)
}
