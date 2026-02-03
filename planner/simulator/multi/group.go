// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"golang.org/x/sync/errgroup"
)

var _ planner.SimulationGroup = (*simGroup)(nil)

type simGroup struct {
	requestRef  planner.RequestRef
	name        string
	simulations []planner.Simulation
	key         planner.SimGroupKey
}

// NewGroup creates a new SimulationGroup with the given name and simulation group key.
func NewGroup(name string, key planner.SimGroupKey, requestRef planner.RequestRef) planner.SimulationGroup {
	return &simGroup{
		name:       name,
		key:        key,
		requestRef: requestRef,
	}
}

func (g *simGroup) Name() string {
	return g.name
}

func (g *simGroup) GetKey() planner.SimGroupKey {
	return g.key
}

func (g *simGroup) GetSimulations() []planner.Simulation {
	return g.simulations
}

func (g *simGroup) AddSimulation(sim planner.Simulation) {
	g.simulations = append(g.simulations, sim)
}

func (g *simGroup) Run(ctx context.Context, getViewFn planner.GetSimulationViewFunc) (runResults []planner.SimulationRunResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: simulation group %q failed: %w", planner.ErrRunSimulationGroup, g.Name(), err)
		}
	}()
	eg, groupCtx := errgroup.WithContext(ctx)
	// TODO: Create pool of similar NodeTemplate + Zone targets to scale and randomize over it so that we can have a balanced allocation across AZ.
	for _, sim := range g.simulations {
		eg.Go(func() error {
			view, err := getViewFn(ctx, fmt.Sprintf("%s_%s", g.requestRef.ID, sim.Name()))
			if err != nil {
				return err
			}
			return sim.Run(groupCtx, view)
		})
	}
	err = eg.Wait()
	if err != nil {
		return
	}

	var simResult planner.SimulationRunResult
	for _, sim := range g.simulations {
		simResult, err = sim.Result()
		if err != nil {
			return
		}
		runResults = append(runResults, simResult)
	}
	return
}

func (g *simGroup) Reset() error {
	resettable := asResettable(g.simulations)
	return ioutil.ResetAll(resettable...)
}

func asResettable(simulations []planner.Simulation) []commontypes.Resettable {
	resettable := make([]commontypes.Resettable, 0, len(simulations))
	for _, s := range simulations {
		resettable = append(resettable, s)
	}
	return resettable
}

// groupSimulations groups the given Simulation instances into one or more SimulationGroups
func groupSimulations(requestRef planner.RequestRef, simulations []planner.Simulation) ([]planner.SimulationGroup, error) {
	groupsByKey := make(map[planner.SimGroupKey]planner.SimulationGroup)
	groupCount := 0
	for _, sim := range simulations {
		gk := planner.SimGroupKey{
			NodePoolPriority:     sim.NodePool().Priority,
			NodeTemplatePriority: sim.NodeTemplate().Priority,
		}
		g, ok := groupsByKey[gk]
		if !ok {
			groupCount++
			name := fmt.Sprintf("sg-%d_%s_%s_%s", groupCount, sim.NodePool().Name, sim.NodeTemplate().Name, gk)
			g = NewGroup(name, gk, requestRef)
		}
		g.AddSimulation(sim)
		groupsByKey[gk] = g
	}
	simGroups := make([]planner.SimulationGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	sortGroups(simGroups)
	return simGroups, nil
}

// sortGroups sorts given simulation groups by NodePool.Priority and then NodeTemplate.Priority.
func sortGroups(groups []planner.SimulationGroup) {
	slices.SortFunc(groups, func(a, b planner.SimulationGroup) int {
		ak := a.GetKey()
		bk := b.GetKey()
		npPriorityCmp := cmp.Compare(ak.NodePoolPriority, bk.NodePoolPriority)
		if npPriorityCmp != 0 {
			return npPriorityCmp
		}
		return cmp.Compare(ak.NodeTemplatePriority, bk.NodeTemplatePriority)
	})
}
