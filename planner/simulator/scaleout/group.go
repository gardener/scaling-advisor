// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaleout

import (
	"context"
	"fmt"
	"slices"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"golang.org/x/sync/errgroup"
)

var _ plannerapi.ScaleOutSimGroup = (*simGroup)(nil)

type simGroup struct {
	requestRef  plannerapi.RequestRef
	name        string
	simulations []plannerapi.ScaleOutSimulation
	key         commontypes.PriorityKey
}

// NewGroup creates a new ScaleOutSimGroup with the given name and simulation group key.
func NewGroup(name string, key commontypes.PriorityKey, requestRef plannerapi.RequestRef) plannerapi.ScaleOutSimGroup {
	return &simGroup{
		name:       name,
		key:        key,
		requestRef: requestRef,
	}
}

func (g *simGroup) Name() string {
	return g.name
}

func (g *simGroup) PriorityKey() commontypes.PriorityKey {
	return g.key
}

func (g *simGroup) GetSimulations() []plannerapi.ScaleOutSimulation {
	return g.simulations
}

func (g *simGroup) AddSimulation(sim plannerapi.ScaleOutSimulation) {
	g.simulations = append(g.simulations, sim)
}

func (g *simGroup) Run(ctx context.Context, getViewFn minkapi.GetViewFunc) (runResults []plannerapi.ScaleOutSimResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q: %w", plannerapi.ErrRunSimulationGroup, g.Name(), err)
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

	var simResult plannerapi.ScaleOutSimResult
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

func asResettable(simulations []plannerapi.ScaleOutSimulation) []commontypes.Resettable {
	resettable := make([]commontypes.Resettable, 0, len(simulations))
	for _, s := range simulations {
		resettable = append(resettable, s)
	}
	return resettable
}

// CreateScaleOutSimGroups groups the given ScaleOutSimulation instances into one or more SimulationGroups
func CreateScaleOutSimGroups(requestRef plannerapi.RequestRef, simulations []plannerapi.ScaleOutSimulation) ([]plannerapi.ScaleOutSimGroup, error) {
	groupsByKey := make(map[commontypes.PriorityKey]plannerapi.ScaleOutSimGroup)
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
	simGroups := make([]plannerapi.ScaleOutSimGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	slices.SortFunc(simGroups, plannerapi.CmpScaleOutSimGroup)
	return simGroups, nil
}
