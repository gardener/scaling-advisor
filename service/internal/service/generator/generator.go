package generator

import (
	"context"
	"fmt"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type Generator struct {
	ctx               context.Context
	log               logr.Logger
	args              *Args
	minKAPIConfig     mkapi.MinKAPIConfig
	minKAPIServer     mkapi.Server
	schedulerLauncher api.SchedulerLauncher
}

type Args struct {
	Scorer            api.NodeScorer
	Selector          api.NodeScoreSelector
	CreateSimFn       api.CreateSimulationFunc
	CreateSimGroupsFn api.CreateSimulationGroupsFunc
	Request           api.ScalingAdviceRequest
	ResponseFn        api.ScalingAdviceResponseFn
}

func New(ctx context.Context, args *Args) *Generator {
	return &Generator{
		ctx:  ctx,
		log:  logr.FromContextOrDiscard(ctx),
		args: args,
	}
}
func (g *Generator) populateBaseView() error {
	// TODO populates the base View with the g.request.ClusterSnapshot
	return nil
}

func (g *Generator) Generate() (err error) {
	defer func() {
		err = api.AsGenerateError(g.args.Request.ID, g.args.Request.CorrelationID, err)
	}()
	if err = g.populateBaseView(); err != nil {
		return
	}

	groups, err := g.createSimulationGroups()
	if err != nil {
		return
	}
	var (
		winnerNodeScores, passNodeScores []api.NodeScore
		unscheduledPods                  []api.PodResourceInfo
	)
	for {
		passNodeScores, unscheduledPods, err = g.RunPass(groups)
		if err != nil {
			return
		}
		if len(passNodeScores) == 0 {
			break
		}
		winnerNodeScores = append(winnerNodeScores, passNodeScores...)
		if len(unscheduledPods) == 0 {
			break
		}
	}

	// If there is no scaling advice then return an error indicating the same.
	if len(winnerNodeScores) == 0 {
		err = api.ErrNoScalingAdvice
		return
	}

	//for _, wns := range winnerNodeScores {
	//	 TODO: create ScaleItems and ScalingAdvice and invoke response function
	//}

	return nil
}

func (g *Generator) RunPass(groups []api.SimulationGroup) (winnerNodeScores []api.NodeScore, unscheduledPods []api.PodResourceInfo, err error) {
	var (
		groupRunResult api.SimGroupRunResult
		groupScores    *api.SimGroupScores
	)
	for _, group := range groups {
		groupRunResult, err = group.Run(g.ctx)
		if err != nil {
			return
		}
		groupScores, err = computeSimGroupScores(g.args.Scorer, g.args.Selector, &groupRunResult)
		if err != nil {
			return
		}
		// TODO: verify logic and error handling flow here with team.
		//if groupScores == nil {
		//	g.log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
		//	continue
		//}
		groupWinner := groupScores.GetWinner()
		if groupWinner == nil {
			g.log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
			continue
		}
		winnerNodeScores = append(winnerNodeScores, *groupWinner)
		if len(groupWinner.UnscheduledPods) == 0 {
			g.log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupRunResult.Name)
		}
	}
	return
}

func computeSimGroupScores(scorer api.NodeScorer, selector api.NodeScoreSelector, groupResult *api.SimGroupRunResult) (*api.SimGroupScores, error) {
	var nodeScores []api.NodeScore
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err := scorer.Compute(sr.NodeScoreArgs)
		if err != nil {
			return nil, fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", api.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerScoreIndex, err := selector(nodeScores)
	if err != nil {
		return nil, fmt.Errorf("%w: node score selection failed for group %q: %w", api.ErrSelectNodeScore, groupResult.Name, err)
	}
	//if winnerScoreIndex < 0 {
	//	return nil, nil //No winning score for this group
	//}
	winnerNode := getScaledNodeOfWinner(groupResult.SimulationResults, winnerScoreIndex)
	//if winnerNode == nil {
	//	return nil, fmt.Errorf("%w: winner node not found for group %q", api.ErrSelectNodeScore, groupResult.Name)
	//}
	return &api.SimGroupScores{
		AllNodeScores:    nodeScores,
		WinnerScoreIndex: winnerScoreIndex,
		WinnerNode:       winnerNode,
	}, nil
}
func getScaledNodeOfWinner(results []api.SimRunResult, winnerScoreIndex int) *corev1.Node {
	var (
		winnerNode *corev1.Node
	)
	for _, sr := range results {
		if sr.NodeScoreArgs.Name == results[winnerScoreIndex].NodeScoreArgs.Name {
			winnerNode = sr.ScaledNode
			break
		}
	}
	return winnerNode
}

// createSimulationGroups creates a slice of SimulationGroup based on priorities that are defined at the NodePool and NodeTemplate level.
func (g *Generator) createSimulationGroups() ([]api.SimulationGroup, error) {
	var (
		allSimulations []api.Simulation
		counter        int
	)
	for _, nodePool := range g.args.Request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				simulationName := fmt.Sprintf("%s-%s-%s-%d", nodePool.Name, zone, nodeTemplate.Name, counter)
				sim, err := g.createSimulation(simulationName, &nodePool, nodeTemplate.Name, zone)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return g.args.CreateSimGroupsFn(allSimulations)
}

func (g *Generator) createSimulation(simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string) (api.Simulation, error) {
	simView, err := g.minKAPIServer.GetSandboxView(g.ctx, simulationName)
	if err != nil {
		return nil, err
	}
	simArgs := &api.SimulationArgs{
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: g.schedulerLauncher,
		View:              simView,
	}
	return g.args.CreateSimFn(simulationName, simArgs)
}
