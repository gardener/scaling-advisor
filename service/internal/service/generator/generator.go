// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type Generator struct {
	args *Args
}

// Args is used to construct a new instance of the Generator
type Args struct {
	ViewAccess        mkapi.ViewAccess
	PricingAccess     svcapi.InstancePricingAccess
	WeightsFn         svcapi.GetWeightsFunc
	NodeScorer        svcapi.NodeScorer
	Selector          svcapi.NodeScoreSelector
	SimulationCreator svcapi.SimulationCreator
	SimulationGrouper svcapi.SimulationGrouper
	SchedulerLauncher svcapi.SchedulerLauncher
}

// RunArgs is used to run the generator and generate scaling advice
// TODO: follow Args, RunArgs convention for all other components too (which have more than 3 parameters) for structural consistency
type RunArgs struct {
	Request   svcapi.ScalingAdviceRequest
	ResultsCh chan<- svcapi.ScalingAdviceResult
	Timeout   time.Duration
}

type SandBoxViewFunc func(log logr.Logger, name string) (mkapi.View, error)

func New(args *Args) *Generator {
	return &Generator{
		args: args,
	}
}

func (g *Generator) Run(ctx context.Context, runArgs *RunArgs) {
	err := g.doGenerate(ctx, runArgs)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
}

func (g *Generator) doGenerate(ctx context.Context, runArgs *RunArgs) (err error) {
	log := logr.FromContextOrDiscard(ctx)

	if err = validateRequest(runArgs.Request); err != nil {
		return
	}

	baseView := g.args.ViewAccess.GetBaseView()
	err = synchronizeBaseView(ctx, baseView, runArgs.Request.Snapshot)
	if err != nil {
		return
	}

	var groupRunPassCounter atomic.Uint32
	groups, err := g.createSimulationGroups(ctx, runArgs, &groupRunPassCounter)
	if err != nil {
		return
	}
	var (
		allWinnerNodeScores []svcapi.NodeScore
		unscheduledPods     []svcapi.PodResourceInfo
	)

	for {
		var passWinnerNodeScores []svcapi.NodeScore
		groupRunPassNum := groupRunPassCounter.Load()
		log := log.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
		passCtx := logr.NewContext(ctx, log)
		passWinnerNodeScores, unscheduledPods, err = g.RunPass(passCtx, groups)
		if err != nil {
			return
		}
		// If there are no winning nodes produced by a pass for the pending unscheduled pods, then abort the loop.
		// This means that we could not identify any node from the node pool and node template combinations (as specified in the constraint)
		// that could accommodate any unscheduled pods. It is fruitless to continue further.
		if len(passWinnerNodeScores) == 0 {
			log.Info("Aborting loop since no node scores produced in %d pass.", groupRunPassNum)
			break
		}
		allWinnerNodeScores = append(allWinnerNodeScores, passWinnerNodeScores...)
		if runArgs.Request.Constraint.Spec.AdviceGenerationMode == sacorev1alpha1.ScalingAdviceGenerationModeIncremental {
			err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassNum, passWinnerNodeScores, unscheduledPods)
			if err != nil {
				return
			}
		}
		if len(unscheduledPods) == 0 {
			log.Info("All pods have been scheduled in %d pass", groupRunPassNum)
			break
		}
		groupRunPassCounter.Add(1)
	}

	// If there is no scaling advice, then return an error indicating the same.
	if len(allWinnerNodeScores) == 0 {
		err = svcapi.ErrNoScalingAdvice
		return
	}
	if runArgs.Request.Constraint.Spec.AdviceGenerationMode == sacorev1alpha1.ScalingAdviceGenerationModeAllAtOnce {
		err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassCounter.Load(), allWinnerNodeScores, unscheduledPods)
	}
	return
}

func validateRequest(request svcapi.ScalingAdviceRequest) error {
	if err := validateConstraint(request.Constraint); err != nil {
		return err
	}
	if err := validateClusterSnapshot(request.Snapshot); err != nil {
		return err
	}
	return nil
}

func validateConstraint(constraint *sacorev1alpha1.ClusterScalingConstraint) error {
	if strings.TrimSpace(constraint.Name) == "" {
		return fmt.Errorf("%w: constraint name must not be empty", svcapi.ErrInvalidScalingConstraint)
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		return fmt.Errorf("%w: constraint namespace must not be empty", svcapi.ErrInvalidScalingConstraint)
	}
	return nil
}

func validateClusterSnapshot(cs *svcapi.ClusterSnapshot) error {
	// Check if all nodes have the required label commonconstants.LabelNodeTemplateName
	for _, nodeInfo := range cs.Nodes {
		if _, ok := nodeInfo.Labels[commonconstants.LabelNodeTemplateName]; !ok {
			return fmt.Errorf("%w: node %q has no label %q", svcapi.ErrMissingRequiredLabel, nodeInfo.Name, commonconstants.LabelNodeTemplateName)
		}
	}
	return nil
}

func synchronizeBaseView(ctx context.Context, view mkapi.View, cs *svcapi.ClusterSnapshot) error {
	// TODO implement delta cluster snapshot to update the base view before every simulation run which will synchronize
	// the base view with the current state of the target cluster.
	view.Reset()
	for _, nodeInfo := range cs.Nodes {
		if _, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo)); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if _, err := view.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod)); err != nil {
			return err
		}
	}
	for _, pc := range cs.PriorityClasses {
		if _, err := view.CreateObject(ctx, typeinfo.PriorityClassesDescriptor.GVK, &pc); err != nil {
			return err
		}
	}
	for _, rc := range cs.RuntimeClasses {
		if _, err := view.CreateObject(ctx, typeinfo.RuntimeClassDescriptor.GVK, &rc); err != nil {
			return err
		}
	}
	return nil

}

func sendScalingAdvice(adviceCh chan<- svcapi.ScalingAdviceResult, request svcapi.ScalingAdviceRequest, groupRunPassNum uint32, winnerNodeScores []svcapi.NodeScore, unscheduledPods []svcapi.PodResourceInfo) error {
	scalingAdvice, err := createScalingAdvice(request, groupRunPassNum, winnerNodeScores, unscheduledPods)
	if err != nil {
		return err
	}
	var msg string
	if request.Constraint.Spec.AdviceGenerationMode == sacorev1alpha1.ScalingAdviceGenerationModeAllAtOnce {
		msg = fmt.Sprintf("%s scaling advice for total num passes %d with %d pending unscheduled pods", request.Constraint.Spec.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
	} else {
		msg = fmt.Sprintf("%s scaling advice for pass %d with %d pending unscheduled pods", request.Constraint.Spec.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
	}

	adviceCh <- svcapi.ScalingAdviceResult{
		Response: &svcapi.ScalingAdviceResponse{
			RequestRef:    request.ScalingAdviceRequestRef,
			Message:       msg,
			ScalingAdvice: scalingAdvice,
		},
	}
	return nil
}

func (g *Generator) RunPass(ctx context.Context, groups []svcapi.SimulationGroup) (winnerNodeScores []svcapi.NodeScore, unscheduledPods []svcapi.PodResourceInfo, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupRunResult svcapi.SimGroupRunResult
		groupScores    svcapi.SimGroupScores
	)
	for _, group := range groups {
		groupRunResult, err = group.Run(ctx)
		if err != nil {
			return
		}
		groupScores, err = computeSimGroupScores(g.args.PricingAccess, g.args.WeightsFn, g.args.NodeScorer, g.args.Selector, &groupRunResult)
		if err != nil {
			return
		}
		if groupScores.WinnerNodeScore == nil {
			log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
			continue
		}
		winnerNodeScores = append(winnerNodeScores, *groupScores.WinnerNodeScore)
		if len(groupScores.WinnerNodeScore.UnscheduledPods) == 0 {
			log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupRunResult.Name)
			break
		}
	}
	return
}

func computeSimGroupScores(pricer svcapi.InstancePricingAccess, weightsFun svcapi.GetWeightsFunc, scorer svcapi.NodeScorer, selector svcapi.NodeScoreSelector, groupResult *svcapi.SimGroupRunResult) (svcapi.SimGroupScores, error) {
	var nodeScores []svcapi.NodeScore
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err := scorer.Compute(sr.NodeScorerArgs)
		if err != nil {
			return svcapi.SimGroupScores{}, fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", svcapi.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerNodeScore, err := selector(nodeScores, weightsFun, pricer)
	if err != nil {
		return svcapi.SimGroupScores{}, fmt.Errorf("%w: node score selection failed for group %q: %w", svcapi.ErrSelectNodeScore, groupResult.Name, err)
	}
	//if winnerScoreIndex < 0 {
	//	return nil, nil //No winning score for this group
	//}
	winnerNode := getScaledNodeOfWinner(groupResult.SimulationResults, winnerNodeScore)
	//if winnerNode == nil {
	//	return nil, fmt.Errorf("%w: winner node not found for group %q", api.ErrSelectNodeScore, groupResult.InstanceType)
	//}
	return svcapi.SimGroupScores{
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
		if sr.NodeScorerArgs.ID == winnerNodeScore.ID {
			winnerNode = sr.ScaledNode
			break
		}
	}
	return winnerNode
}

// createSimulationGroups creates a slice of SimulationGroup based on priorities that are defined at the NodePool and NodeTemplate level.
func (g *Generator) createSimulationGroups(ctx context.Context, runArgs *RunArgs, groupRunPassCounter *atomic.Uint32) ([]svcapi.SimulationGroup, error) {
	request := runArgs.Request
	var (
		allSimulations []svcapi.Simulation
	)
	for _, nodePool := range request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				simulationName := fmt.Sprintf("%s_%s_%s", nodePool.Name, nodeTemplate.Name, zone)

				sim, err := g.createSimulation(ctx, simulationName, &nodePool, nodeTemplate.Name, zone, groupRunPassCounter)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return g.args.SimulationGrouper.Group(allSimulations)
}

func (g *Generator) createSimulation(ctx context.Context, simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string, groupRunPassCounter *atomic.Uint32) (svcapi.Simulation, error) {
	simView, err := g.args.ViewAccess.GetOrCreateSandboxView(ctx, simulationName)
	if err != nil {
		return nil, err
	}
	simArgs := &svcapi.SimulationArgs{
		GroupRunPassCounter: groupRunPassCounter,
		AvailabilityZone:    zone,
		NodePool:            nodePool,
		NodeTemplateName:    nodeTemplateName,
		SchedulerLauncher:   g.args.SchedulerLauncher,
		View:                simView,
		TrackPollInterval:   10 * time.Millisecond,
	}
	return g.args.SimulationCreator.Create(simulationName, simArgs)
}
