package planner

import (
	"github.com/gardener/scaling-advisor/planner/simulation"
	"github.com/gardener/scaling-advisor/planner/simulator"
	"github.com/gardener/scaling-advisor/planner/weigher"

	plannerapi "github.com/gardener/scaling-advisor/api/planner"
)

var (
	_ plannerapi.ScalingPlannerFactory = (*defaultFactory)(nil)
)

// NewFactories returns a instance of plannerapi.Factories populated with implementation of factory facades.
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
