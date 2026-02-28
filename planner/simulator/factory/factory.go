// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package factory provides implementation of SimulatorFactory
package factory

import (
	"fmt"

	"github.com/gardener/scaling-advisor/planner/simulator/scaleout/singlenode"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
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
	case commontypes.SimulatorStrategySingleNodeMultiSim:
		return singlenode.New(args)
	case commontypes.SimulatorStrategyMultiNodeSingleSim:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", commonerrors.ErrUnimplemented, args.Strategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", plannerapi.ErrUnsupportedSimulatorStrategy, args.Strategy)
	}
}
