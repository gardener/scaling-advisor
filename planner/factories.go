package planner

import (
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	simulatorfactory "github.com/gardener/scaling-advisor/planner/simulator/factory"

	simulationfactory "github.com/gardener/scaling-advisor/simulation/factory"
)

var (
	_ plannerapi.ScalingPlannerFactory = (*defaultFactory)(nil)
)

// NewFactories returns an instance of plannerapi.Factories populated with implementation of factory facades.
func NewFactories() plannerapi.Factories {
	return plannerapi.Factories{
		Planner:    &defaultFactory{},
		Simulator:  simulatorfactory.New(),
		Simulation: simulationfactory.New(),
	}
}

type defaultFactory struct{}

// NewPlanner creates a new instance of the default ScalingPlanner using the provided Args.
func (f *defaultFactory) NewPlanner(args plannerapi.ScalingPlannerArgs) (plannerapi.ScalingPlanner, error) {
	return NewPlanner(args)
}
