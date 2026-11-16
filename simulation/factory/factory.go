// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package factory provides implementation of SimulationFactory
package factory

import (
	"github.com/gardener/scaling-advisor/simulation/scaleout"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
)

var _ simulationapi.SimulationFactory = (*defaultFactory)(nil)

// New returns a default instance of SimulationFactory which in turn can be used to construct scale-out and scale-in simulation's.
func New() simulationapi.SimulationFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) New(args simulationapi.SimulationArgs) (simulationapi.Simulation, error) {
	return scaleout.NewDefault(args)
}
