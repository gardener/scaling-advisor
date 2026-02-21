package simulation

import (
	"github.com/gardener/scaling-advisor/planner/simulation/scaleout"

	plannerapi "github.com/gardener/scaling-advisor/api/planner"
)

var _ plannerapi.SimulationFactory = (*defaultFactory)(nil)

// NewFactory returns a default instance of SimulationFactory which in turn can be used to construct scale-out and scale-in simulation's.
func NewFactory() plannerapi.SimulationFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) NewScaleOut(name string, args plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	return scaleout.NewDefault(name, args)
}
