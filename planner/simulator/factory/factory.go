// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package factory provides implementation of SimulatorFactory
package factory

import (
	"fmt"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/gardener/scaling-advisor/planner/simulator/scaleout/singlenode"

	apicommon "github.com/gardener/scaling-advisor/api/common"
)

var _ plannerapi.SimulatorFactory = (*defaultFactory)(nil)

// New returns a default instance of SimulatorFactory, which in turn can be used to create plannerapi.ScaleOutSimulator or
// plannerapi.ScaleInSimulator
func New() plannerapi.SimulatorFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) GetScaleOutSimulator(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	switch args.Strategy {
	case "":
		return nil, fmt.Errorf("%w: simulation strategy must be specified", plannerapi.ErrCreateSimulator)
	case apicommon.SimulatorStrategySingleNodeMultiSim:
		return singlenode.New(args)
	case apicommon.SimulatorStrategyMultiNodeSingleSim:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", plannerapi.ErrCreateSimulator, args.Strategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", plannerapi.ErrCreateSimulator, args.Strategy)
	}
}

func (s *defaultFactory) GetScaleInSimulator(_ plannerapi.SimulatorArgs) (plannerapi.ScaleInSimulator, error) {
	return nil, fmt.Errorf("%w: scale-in simulator not yet implemented", plannerapi.ErrCreateSimulator)
}
