// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"cmp"
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"slices"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"golang.org/x/sync/errgroup"
)

var _ planner.ScaleOutSimGroup = (*simGroup)(nil)

type simGroup struct {
	requestRef  planner.RequestRef
	name        string
	simulations []planner.ScaleOutSimulation
	key         planner.PriorityKey
}

// NewGroup creates a new ScaleOutSimGroup with the given name and simulation group key.
func NewGroup(name string, key planner.PriorityKey, requestRef planner.RequestRef) planner.ScaleOutSimGroup {
	return &simGroup{
		name:       name,
		key:        key,
		requestRef: requestRef,
	}
}

func (g *simGroup) Name() string {
	return g.name
}

func (g *simGroup) GetKey() planner.PriorityKey {
	return g.key
}

func (g *simGroup) GetSimulations() []planner.ScaleOutSimulation {
	return g.simulations
}

func (g *simGroup) AddSimulation(sim planner.ScaleOutSimulation) {
	g.simulations = append(g.simulations, sim)
}

func (g *simGroup) Run(ctx context.Context, getViewFn minkapi.ViewFactory) (runResults []planner.ScaleOutSimResult, err error) {
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

	var simResult planner.ScaleOutSimResult
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

func asResettable(simulations []planner.ScaleOutSimulation) []commontypes.Resettable {
	resettable := make([]commontypes.Resettable, 0, len(simulations))
	for _, s := range simulations {
		resettable = append(resettable, s)
	}
	return resettable
}

// groupSimulations groups the given ScaleOutSimulation instances into one or more SimulationGroups
func groupSimulations(requestRef planner.RequestRef, simulations []planner.ScaleOutSimulation) ([]planner.ScaleOutSimGroup, error) {
	groupsByKey := make(map[planner.PriorityKey]planner.ScaleOutSimGroup)
	groupCount := 0
	for _, sim := range simulations {
		pk := sim.PriorityKey()
		g, ok := groupsByKey[pk]
		if !ok {
			groupCount++
			name := fmt.Sprintf("sg-%d_%s", groupCount, pk.String())
			g = NewGroup(name, pk, requestRef)
		}
		g.AddSimulation(sim)
		groupsByKey[pk] = g
	}
	simGroups := make([]planner.ScaleOutSimGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	sortGroups(simGroups)
	return simGroups, nil
}

// sortGroups sorts given simulation groups by NodePool.Priority and then NodeTemplate.Priority.
func sortGroups(groups []planner.ScaleOutSimGroup) {
	slices.SortFunc(groups, func(a, b planner.ScaleOutSimGroup) int {
		ak := a.GetKey()
		bk := b.GetKey()
		npPriorityCmp := cmp.Compare(ak.NodePoolPriority, bk.NodePoolPriority)
		if npPriorityCmp != 0 {
			return npPriorityCmp
		}
		return cmp.Compare(ak.NodeTemplatePriority, bk.NodeTemplatePriority)
	})
}
