package planner

import (
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/simulation"
	"github.com/gardener/scaling-advisor/planner/simulator"
	"github.com/gardener/scaling-advisor/planner/weigher"
)

var (
	_ plannerapi.ScalingPlannerFactory = (*defaultFactory)(nil)
)

func NewFactories() plannerapi.Factories {
	return plannerapi.Factories{
		Planner:         &defaultFactory{},
		Simulator:       simulator.NewFactory(),
		Simulation:      simulation.NewFactory(),
		ResourceWeigher: weigher.New(),
	}
}

type defaultFactory struct{}

// NewPlanner creates a new instance of the default ScalingPlanner using the provided Args.
func (f *defaultFactory) NewPlanner(args plannerapi.ScalingPlannerArgs) (plannerapi.ScalingPlanner, error) {
	return NewPlanner(args)
}
