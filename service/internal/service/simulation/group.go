// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package simulation

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	svcapi "github.com/gardener/scaling-advisor/api/service"
	"golang.org/x/sync/errgroup"
)

var _ svcapi.SimulationGroup = (*defaultSimulationGroup)(nil)

type defaultSimulationGroup struct {
	name        string
	key         svcapi.SimGroupKey
	simulations []svcapi.Simulation
}

func CreateSimulationGroups(simulations []svcapi.Simulation) ([]svcapi.SimulationGroup, error) {
	groupsByKey := make(map[svcapi.SimGroupKey]*defaultSimulationGroup)
	for _, sim := range simulations {
		gk := svcapi.SimGroupKey{
			NodePoolPriority:     sim.NodePool().Priority,
			NodeTemplatePriority: sim.NodeTemplate().Priority,
		}
		g, ok := groupsByKey[gk]
		if !ok {
			g = &defaultSimulationGroup{
				name:        fmt.Sprintf("%s_%s_%s", sim.NodePool().Name, sim.NodeTemplate().Name, gk),
				key:         gk,
				simulations: []svcapi.Simulation{sim},
			}
		} else {
			g.simulations = append(g.simulations, sim)
		}
		groupsByKey[gk] = g
	}
	simGroups := make([]svcapi.SimulationGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	SortGroups(simGroups)
	return simGroups, nil
}

func (g *defaultSimulationGroup) Name() string {
	return g.name
}

func (g *defaultSimulationGroup) GetKey() svcapi.SimGroupKey {
	return g.key
}

func (g *defaultSimulationGroup) GetSimulations() []svcapi.Simulation {
	return g.simulations
}

func (g *defaultSimulationGroup) Run(ctx context.Context) (result svcapi.SimGroupRunResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: simulation group %q failed: %w", svcapi.ErrRunSimulationGroup, g.Name(), err)
		}
	}()
	eg, groupCtx := errgroup.WithContext(ctx)
	// TODO: Create pool of similar NodeTemplate + Zone targets to scale and randomize over it so that we can have a balanced allocation across AZ.
	for _, sim := range g.simulations {
		eg.Go(func() error {
			return sim.Run(groupCtx)
		})
	}
	err = eg.Wait()
	if err != nil {
		return
	}

	var simResults []svcapi.SimRunResult
	var simResult svcapi.SimRunResult
	for _, sim := range g.simulations {
		simResult, err = sim.Result()
		if err != nil {
			return
		}
		simResults = append(simResults, simResult)
	}
	result = svcapi.SimGroupRunResult{
		Name:              g.name,
		Key:               g.key,
		SimulationResults: simResults,
	}
	return
}

func SortGroups(groups []svcapi.SimulationGroup) {
	slices.SortFunc(groups, func(a, b svcapi.SimulationGroup) int {
		ak := a.GetKey()
		bk := b.GetKey()
		npPriorityCmp := cmp.Compare(ak.NodePoolPriority, bk.NodePoolPriority)
		if npPriorityCmp != 0 {
			return npPriorityCmp
		}
		return cmp.Compare(ak.NodeTemplatePriority, bk.NodeTemplatePriority)
	})
}
