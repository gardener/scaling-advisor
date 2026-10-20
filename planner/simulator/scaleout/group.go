// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package scaleout

import (
	"context"
	"fmt"
	"slices"
	"sync"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"

	apicomponent "github.com/gardener/scaling-advisor/api/common/component"
	"github.com/gardener/scaling-advisor/common/ioutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	"golang.org/x/sync/errgroup"
)

var _ simulationapi.SimulationGroup = (*simGroup)(nil)

type simGroup struct {
	requestRef  plannerapi.RequestRef
	name        string
	simulations []simulationapi.Simulation
	key         simulationapi.GroupKey
}

// NewGroup creates a new SimulationGroup with the given name and simulation group key.
func NewGroup(name string, key simulationapi.GroupKey, requestRef plannerapi.RequestRef) simulationapi.SimulationGroup {
	return &simGroup{
		name:       name,
		key:        key,
		requestRef: requestRef,
	}
}

func (g *simGroup) Name() string {
	return g.name
}

func (g *simGroup) Key() simulationapi.GroupKey {
	return g.key
}

func (g *simGroup) GetSimulations() []simulationapi.Simulation {
	return g.simulations
}

func (g *simGroup) AddSimulation(sim simulationapi.Simulation) {
	g.simulations = append(g.simulations, sim)
}

func (g *simGroup) Run(ctx context.Context, getViewFn minkapi.GetViewFunc) (runResults []simulationapi.SimulationResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q: %w", plannerapi.ErrRunSimulationGroup, g.Name(), err)
		}
	}()
	eg, groupCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	results := make([]simulationapi.SimulationResult, 0, len(g.simulations))
	// TODO: Create pool of similar NodeTemplate + Zone targets to scale and randomize over it so that we can have a balanced allocation across AZ.
	for _, sim := range g.simulations {
		sim := sim
		eg.Go(func() error {
			view, err := getViewFn(ctx, fmt.Sprintf("%s_%s", g.requestRef.ID, sim.Name()))
			if err != nil {
				return err
			}
			result, err := sim.Run(groupCtx, view)
			if err != nil {
				return err
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		return
	}
	runResults = results
	return
}

func (g *simGroup) Reset() error {
	resettable := asResettable(g.simulations)
	return ioutil.ResetAll(resettable...)
}

func asResettable(simulations []simulationapi.Simulation) []apicomponent.Resettable {
	resettable := make([]apicomponent.Resettable, 0, len(simulations))
	for _, s := range simulations {
		resettable = append(resettable, s)
	}
	return resettable
}

// CreateScaleOutSimGroups groups the given Simulation instances into one or more SimulationGroups
func CreateScaleOutSimGroups(requestRef plannerapi.RequestRef, simulations []simulationapi.Simulation) ([]simulationapi.SimulationGroup, error) {
	groupsByKey := make(map[simulationapi.GroupKey]simulationapi.SimulationGroup)
	groupCount := 0
	for _, sim := range simulations {
		pk := sim.GroupKey()
		g, ok := groupsByKey[pk]
		if !ok {
			groupCount++
			name := fmt.Sprintf("sg-%d_%s", groupCount, pk.String())
			g = NewGroup(name, pk, requestRef)
		}
		g.AddSimulation(sim)
		groupsByKey[pk] = g
	}
	simGroups := make([]simulationapi.SimulationGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	slices.SortFunc(simGroups, simulationapi.CmpSimulationGroup)
	return simGroups, nil
}
