// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package factory provides implementation of SimulationFactory
package factory

import (
	"github.com/gardener/scaling-advisor/planner/simulation/scaleout"

	plannerapi "github.com/gardener/scaling-advisor/api/planner"
)

var _ plannerapi.SimulationFactory = (*defaultFactory)(nil)

// New returns a default instance of SimulationFactory which in turn can be used to construct scale-out and scale-in simulation's.
func New() plannerapi.SimulationFactory {
	return &defaultFactory{}
}

type defaultFactory struct{}

func (s *defaultFactory) NewScaleOut(args plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	return scaleout.NewDefault(args)
}
