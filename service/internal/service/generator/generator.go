// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sync/atomic"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Generator is responsible for creating and managing simulations to generate scaling advice.
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
	LogBaseDir        string
}

// RunArgs is used to run the generator and generate scaling advice
// TODO: follow Args, RunArgs convention for all other components too (which have more than 3 parameters) for structural consistency
type RunArgs struct {
	ResultsCh chan<- svcapi.ScalingAdviceResult
	Request   svcapi.ScalingAdviceRequest
	Timeout   time.Duration
}

// New creates a new instance of Generator using the provided Args. It initializes the Generator struct.
func New(args *Args) *Generator {
	return &Generator{
		args: args,
	}
}

// Run executes the scaling advice generation process using the provided context and runArgs.
// The results of the generation process are sent as one more ScalingAdviceResult's on the RunArgs.ResultsCh channel.
func (g *Generator) Run(ctx context.Context, runArgs *RunArgs) {
	genCtx, logPath, logCloser, err := wrapGenerationContext(ctx, g.args.LogBaseDir, runArgs.Request.ID, runArgs.Request.CorrelationID, runArgs.Request.EnableDiagnostics)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
	defer ioutil.CloseQuietly(logCloser)
	err = g.doGenerate(genCtx, runArgs, logPath)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
}

func (g *Generator) doGenerate(ctx context.Context, runArgs *RunArgs, logPath string) (err error) {
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
	allWinnerNodeScores, unscheduledPods, err := g.runPasses(ctx, runArgs, groups, &groupRunPassCounter, logPath)
	if err != nil {
		return
	}
	if len(allWinnerNodeScores) == 0 {
		log.Info("No scaling advice generated. No winning nodes produced by any simulation group.")
		err = svcapi.ErrNoScalingAdvice
		return
	}
	if runArgs.Request.Constraint.Spec.AdviceGenerationMode == sacorev1alpha1.ScalingAdviceGenerationModeAllAtOnce {
		err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassCounter.Load(), allWinnerNodeScores, unscheduledPods, logPath)
	}
	return
}

// runPasses FIXME TODO needs to be refactored into separate Passes abstraction to avoid so many arguments being passed.
func (g *Generator) runPasses(ctx context.Context, runArgs *RunArgs, groups []svcapi.SimulationGroup, groupRunPassCounter *atomic.Uint32, logPath string) (allWinnerNodeScores []svcapi.NodeScore, unscheduledPods []types.NamespacedName, err error) {
	log := logr.FromContextOrDiscard(ctx)
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			log.Info("Generator context done. Aborting pass loop", "err", err)
			return
		default:
			var passWinnerNodeScores []svcapi.NodeScore
			groupRunPassNum := groupRunPassCounter.Load()
			log := log.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
			passCtx := logr.NewContext(ctx, log)
			passWinnerNodeScores, unscheduledPods, err = g.runPass(passCtx, groups)
			if err != nil {
				return
			}
			// If there are no winning nodes produced by a pass for the pending unscheduled pods, then abort the loop.
			// This means that we could not identify any node from the node pool and node template combinations (as specified in the constraint)
			// that could accommodate any unscheduled pods. It is fruitless to continue further.
			if len(passWinnerNodeScores) == 0 {
				log.Info("Aborting loop since no node scores produced in pass.", "groupRunPass", groupRunPassNum)
				return
			}
			allWinnerNodeScores = append(allWinnerNodeScores, passWinnerNodeScores...)
			if runArgs.Request.Constraint.Spec.AdviceGenerationMode == sacorev1alpha1.ScalingAdviceGenerationModeIncremental {
				err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassNum, passWinnerNodeScores, unscheduledPods, logPath)
				if err != nil {
					return
				}
			}
			if len(unscheduledPods) == 0 {
				log.Info("All pods have been scheduled in pass", "groupRunPass", groupRunPassNum)
				return
			}
			groupRunPassCounter.Add(1)
		}
	}
}

func validateRequest(request svcapi.ScalingAdviceRequest) error {
	if err := validateConstraint(request.Constraint, field.NewPath("request.constraint")); err != nil {
		return err
	}
	if err := validateClusterSnapshot(request.Snapshot); err != nil {
		return err
	}
	return nil
}

func validateConstraint(constraint *sacorev1alpha1.ClusterScalingConstraint, fldPath *field.Path) error {
	errList := sacorev1alpha1.ValidateClusterScalingConstraint(constraint, fldPath)
	if len(errList) > 0 {
		return fmt.Errorf("%w: %w", svcapi.ErrInvalidScalingConstraint, errList.ToAggregate())
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

// sendScalingAdvice needs to be fixed: FIXME, TODO: reduce num of args by making this a method or wrapping args into struct or alternative
func sendScalingAdvice(adviceCh chan<- svcapi.ScalingAdviceResult, request svcapi.ScalingAdviceRequest, groupRunPassNum uint32, winnerNodeScores []svcapi.NodeScore, unscheduledPods []types.NamespacedName, logPath string) error {
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

	response := svcapi.ScalingAdviceResponse{
		RequestRef:    request.ScalingAdviceRequestRef,
		Message:       msg,
		ScalingAdvice: scalingAdvice,
	}

	if request.EnableDiagnostics {
		response.Diagnostics = &sacorev1alpha1.ScalingAdviceDiagnostic{
			SimRunResults: nil, // TODO: populate SimRunResults
			TraceLogURL:   logPath,
		}
	}

	adviceCh <- svcapi.ScalingAdviceResult{
		Response: &response,
	}

	return nil
}

func (g *Generator) runPass(ctx context.Context, groups []svcapi.SimulationGroup) (winnerNodeScores []svcapi.NodeScore, unscheduledPods []types.NamespacedName, err error) {
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
		unscheduledPods = groupScores.WinnerNodeScore.UnscheduledPods
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

func getScaledNodeOfWinner(simRunResults []svcapi.SimRunResult, winnerNodeScore *svcapi.NodeScore) *corev1.Node {
	var (
		winnerNode *corev1.Node
	)
	for _, sr := range simRunResults {
		if sr.ID == winnerNodeScore.ID {
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

func wrapGenerationContext(ctx context.Context, baseLogDir, requestID, correlationID string, enableDiagnostics bool) (genCtx context.Context, logPath string, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", requestID, "correlationID", correlationID))
	if enableDiagnostics {
		if baseLogDir == "" {
			baseLogDir = os.TempDir()
		}
		logPath = path.Join(baseLogDir, fmt.Sprintf("%s-%s.log", correlationID, requestID))
		genCtx, logCloser, err = logutil.WrapContextWithFileLogger(genCtx, correlationID, logPath)
		log := logr.FromContextOrDiscard(genCtx)
		log.Info("Diagnostics enabled for this request", "logPath", logPath)
	}
	return
}
