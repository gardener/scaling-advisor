package simulation

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/simulation/scaleout"
)

var _ plannerapi.SimulationFactory = (*defaultFactory)(nil)

func NewFactory() plannerapi.SimulationFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) NewScaleOut(name string, args plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	return scaleout.NewDefault(name, args)
}
