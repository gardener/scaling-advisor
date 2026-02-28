// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package mnsm provides implementation and helper routines of a ScaleOutSimulator that performs simulations that scale
// multiple nodes at a time.
package multinode

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/planner/simulator/scaleout"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
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
	s.state = scaleout.RequestStateWith(request, simulationFactory, s.viewAccess)
	go func() {
		defer close(s.state.ResultCh)
		if err := s.doSimulate(ctx); err != nil {
			scaleout.SendPlanError(s.state.ResultCh, request.GetRef(), err)
		}
	}()
	return s.state.ResultCh
}

func (s *simulatorSingleSim) doSimulate(ctx context.Context) (err error) {
	if err = s.state.Initialize(ctx); err != nil {
		return
	}
	err = fmt.Errorf("%w: to be implemented", commonerrors.ErrUnimplemented)
	return
}
