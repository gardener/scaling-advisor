// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"context"
	"fmt"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type Generator struct {
	args              *Args
	schedulerLauncher svcapi.SchedulerLauncher
}

// Args is used to construct a new instance of the Generator
type Args struct {
	Pricer                   svcapi.InstanceTypeInfoAccess
	WeightsFn                svcapi.GetWeightsFunc
	Scorer                   svcapi.NodeScorer
	Selector                 svcapi.NodeScoreSelector
	CreateSimFn              svcapi.CreateSimulationFunc
	CreateSimGroupsFn        svcapi.CreateSimulationGroupsFunc
	SchedulerConfigPath      string
	MaxConcurrentSimulations int
	// Option 1
	// GetBaseViewFn  func() mkapi.View
	// GetSandboxViewFn func(name)  mkapi.View
}

// RunArgs is used to run the generator and generate scaling advice
// TODO: follow Args, RunArgs convention for all other components too (which have more than 3 parameters) for structural consistency
type RunArgs struct {
	BaseView      mkapi.View
	SandboxViewFn SandBoxViewFunc
	Request       svcapi.ScalingAdviceRequest
	AdviceEventCh chan<- svcapi.ScalingAdviceEvent
}

type SandBoxViewFunc func(log logr.Logger, name string) (mkapi.View, error)

func New(args *Args) (*Generator, error) {
	launcher, err := scheduler.NewLauncher(args.SchedulerConfigPath, args.MaxConcurrentSimulations)
	if err != nil {
		return nil, err
	}
	return &Generator{
		args:              args,
		schedulerLauncher: launcher,
	}, nil
}

func (g *Generator) Run(ctx context.Context, runArgs *RunArgs) {
	err := g.doGenerate(ctx, runArgs)
	if err != nil {
		runArgs.AdviceEventCh <- svcapi.ScalingAdviceEvent{
			Err: svcapi.AsGenerateError(runArgs.Request.ID, runArgs.Request.CorrelationID, err),
		}
		return
	}
}

func (g *Generator) doGenerate(ctx context.Context, runArgs *RunArgs) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	groups, err := g.createSimulationGroups(log, runArgs)
	if err != nil {
		return
	}
	var (
		winnerNodeScores, passNodeScores []svcapi.NodeScore
		unscheduledPods                  []svcapi.PodResourceInfo
	)
	for {
		passNodeScores, unscheduledPods, err = g.RunPass(ctx, groups)
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

	// If there is no scaling advice, then return an error indicating the same.
	if len(winnerNodeScores) == 0 {
		err = svcapi.ErrNoScalingAdvice
		return
	}

	//for _, wns := range winnerNodeScores {
	//	 TODO: create ScaleItems and ScalingAdvice and ScaleEvent and sent on event adviceCh
	//}
	return nil

}

func (g *Generator) RunPass(ctx context.Context, groups []svcapi.SimulationGroup) (winnerNodeScores []svcapi.NodeScore, unscheduledPods []svcapi.PodResourceInfo, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupRunResult svcapi.SimGroupRunResult
		groupScores    *svcapi.SimGroupScores
	)
	for _, group := range groups {
		groupRunResult, err = group.Run(ctx)
		if err != nil {
			return
		}
		groupScores, err = computeSimGroupScores(g.args.Pricer, g.args.WeightsFn, g.args.Scorer, g.args.Selector, &groupRunResult)
		if err != nil {
			return
		}
		// TODO: verify logic and error handling flow here with team.
		//if groupScores == nil {
		//	g.log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
		//	continue
		//}
		if groupScores.WinnerNodeScore == nil {
			log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
			continue
		}
		winnerNodeScores = append(winnerNodeScores, *groupScores.WinnerNodeScore)
		if len(groupScores.WinnerNodeScore.UnscheduledPods) == 0 {
			log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupRunResult.Name)
		}
	}
	return
}

func computeSimGroupScores(pricer svcapi.InstanceTypeInfoAccess, weightsFun svcapi.GetWeightsFunc, scorer svcapi.NodeScorer, selector svcapi.NodeScoreSelector, groupResult *svcapi.SimGroupRunResult) (*svcapi.SimGroupScores, error) {
	var nodeScores []svcapi.NodeScore
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err := scorer.Compute(sr.NodeScoreArgs)
		if err != nil {
			// TODO: fix this when compute already returns a error with a sentinel wrapped error.
			return nil, fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", svcapi.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerNodeScore, err := selector(nodeScores, weightsFun, pricer)
	if err != nil {
		return nil, fmt.Errorf("%w: node score selection failed for group %q: %w", svcapi.ErrSelectNodeScore, groupResult.Name, err)
	}
	//if winnerScoreIndex < 0 {
	//	return nil, nil //No winning score for this group
	//}
	winnerNode := getScaledNodeOfWinner(groupResult.SimulationResults, winnerNodeScore)
	//if winnerNode == nil {
	//	return nil, fmt.Errorf("%w: winner node not found for group %q", api.ErrSelectNodeScore, groupResult.Name)
	//}
	return &svcapi.SimGroupScores{
		AllNodeScores:   nodeScores,
		WinnerNodeScore: winnerNodeScore,
		WinnerNode:      winnerNode,
	}, nil
}
func getScaledNodeOfWinner(results []svcapi.SimRunResult, winnerNodeScore *svcapi.NodeScore) *corev1.Node {
	var (
		winnerNode *corev1.Node
	)
	for _, sr := range results {
		if sr.NodeScoreArgs.ID == winnerNodeScore.ID {
			winnerNode = sr.ScaledNode
			break
		}
	}
	return winnerNode
}

// createSimulationGroups creates a slice of SimulationGroup based on priorities that are defined at the NodePool and NodeTemplate level.
func (g *Generator) createSimulationGroups(log logr.Logger, runArgs *RunArgs) ([]svcapi.SimulationGroup, error) {
	request := runArgs.Request
	var (
		allSimulations []svcapi.Simulation
	)
	for _, nodePool := range request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				simulationName := fmt.Sprintf("%s-%s-%s", nodePool.Name, nodeTemplate.Name, zone)
				sim, err := g.createSimulation(log, runArgs.SandboxViewFn, simulationName, &nodePool, nodeTemplate.Name, zone)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return g.args.CreateSimGroupsFn(allSimulations)
}

func (g *Generator) createSimulation(log logr.Logger, sandboxViewFn SandBoxViewFunc, simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string) (svcapi.Simulation, error) {
	simView, err := sandboxViewFn(log, simulationName)
	if err != nil {
		return nil, err
	}
	simArgs := &svcapi.SimulationArgs{
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: g.schedulerLauncher,
		View:              simView,
	}
	return g.args.CreateSimFn(simulationName, simArgs)
}
