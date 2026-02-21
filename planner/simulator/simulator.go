// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/simulator/multi"
)

var _ plannerapi.SimulatorFactory = (*defaultFactory)(nil)

func NewFactory() plannerapi.SimulatorFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) GetScaleOutSimulator(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	switch args.Strategy {
	case "":
		return nil, fmt.Errorf("%w: simulation strategy must be specified", plannerapi.ErrCreateSimulator)
	case commontypes.SimulatorStrategySingleNodeMultiSim:
		return multi.NewScaleOutSimulator(args)
	case commontypes.SimulatorStrategyMultiNodeSingleSim:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", commonerrors.ErrUnimplemented, args.Strategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", plannerapi.ErrUnsupportedSimulatorStrategy, args.Strategy)
	}
}
