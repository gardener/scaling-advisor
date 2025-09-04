package simulation

import (
	"cmp"
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/service/api"
	"golang.org/x/sync/errgroup"
	"slices"
)

var _ api.SimulationGroup = (*defaultSimulationGroup)(nil)

type defaultSimulationGroup struct {
	name        string
	key         api.SimGroupKey
	simulations []api.Simulation
}

func CreateSimulationGroups(simulations []api.Simulation) ([]api.SimulationGroup, error) {
	groupsByKey := make(map[api.SimGroupKey]*defaultSimulationGroup)
	for _, sim := range simulations {
		gk := api.SimGroupKey{
			NodePoolPriority:     sim.NodePool().Priority,
			NodeTemplatePriority: sim.NodeTemplate().Priority,
		}
		g, ok := groupsByKey[gk]
		if !ok {
			g = &defaultSimulationGroup{
				name:        fmt.Sprintf("%s_%s_%s", sim.NodePool().Name, sim.NodeTemplate().Name, gk),
				key:         gk,
				simulations: []api.Simulation{sim},
			}
		} else {
			g.simulations = append(g.simulations, sim)
		}
		groupsByKey[gk] = g
	}
	simGroups := make([]api.SimulationGroup, 0, len(groupsByKey))
	for _, g := range groupsByKey {
		simGroups = append(simGroups, g)
	}
	SortGroups(simGroups)
	return simGroups, nil
}

func (g *defaultSimulationGroup) Name() string {
	return g.name
}
func (g *defaultSimulationGroup) GetKey() api.SimGroupKey {
	return g.key
}

func (g *defaultSimulationGroup) GetSimulations() []api.Simulation {
	return g.simulations
}

func (g *defaultSimulationGroup) Run(ctx context.Context) (result api.SimGroupRunResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: simulation group %q failed: %w", api.ErrRunSimulationGroup, g.Name(), err)
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

	var simResults []api.SimRunResult
	var simResult api.SimRunResult
	for _, sim := range g.simulations {
		simResult, err = sim.Result()
		if err != nil {
			return
		}
		simResults = append(simResults, simResult)
	}
	result = api.SimGroupRunResult{
		Name:              g.name,
		Key:               g.key,
		SimulationResults: simResults,
	}
	return
}

func SortGroups(groups []api.SimulationGroup) {
	slices.SortFunc(groups, func(a, b api.SimulationGroup) int {
		ak := a.GetKey()
		bk := b.GetKey()
		npPriorityCmp := cmp.Compare(ak.NodePoolPriority, bk.NodePoolPriority)
		if npPriorityCmp != 0 {
			return npPriorityCmp
		}
		return cmp.Compare(ak.NodeTemplatePriority, bk.NodeTemplatePriority)
	})
}
