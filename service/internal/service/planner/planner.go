// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sync/atomic"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/service/internal/service/simulator/multi"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Planner is responsible for creating and managing simulations to generate scaling advice plans.
type Planner struct {
	args *Args
}

// Args is used to construct a new instance of the Planner
type Args struct {
	ViewAccess      minkapi.ViewAccess
	PricingAccess   service.InstancePricingAccess
	ResourceWeigher service.ResourceWeigher
	//SimulationGrouper service.SimulationGrouper
	SchedulerLauncher service.SchedulerLauncher
	LogBaseDir        string
}

// RunArgs is used to run the planner and generate scaling advice
type RunArgs struct {
	ResultsCh chan<- service.ScalingAdviceResult
	Request   service.ScalingAdviceRequest
}

// New creates a new instance of Planner using the provided Args. It initializes the Planner struct.
func New(args *Args) *Planner {
	return &Planner{
		args: args,
	}
}

// Run executes the scaling advice generation process using the provided context and runArgs.
// The results of the generation process are sent as one more ScalingAdviceResult's on the RunArgs.ResultsCh channel.
func (p *Planner) Run(ctx context.Context, runArgs *RunArgs) {
	genCtx, logPath, logCloser, err := wrapGenerationContext(ctx, p.args.LogBaseDir, runArgs.Request.ID, runArgs.Request.CorrelationID, runArgs.Request.EnableDiagnostics)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
	defer ioutil.CloseQuietly(logCloser)

	simulator, err := p.getSimulator(&runArgs.Request)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
	planConsumerFn := func(scaleOutPlan sacorev1alpha1.ScaleOutPlan) error {
		resp := &service.ScalingAdviceResponse{
			ScalingAdvice: nil,
			Diagnostics:   nil,
			RequestRef:    service.ScalingAdviceRequestRef{},
			Message:       "",
		}
		runArgs.ResultsCh <- service.ScalingAdviceResult{
			Response: resp,
		}
	}
	simulator.SimulateScaleOut(ctx, planConsumerFn)
	err = p.doGenerate(genCtx, runArgs, logPath)
	if err != nil {
		SendError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
		return
	}
}

func (p *Planner) getSimulator(req *service.ScalingAdviceRequest) (service.Simulator, error) {
	switch req.SimulationStrategy {
	case commontypes.SimulationStrategyMultiSimulationsPerGroup:
		return multi.NewSimulator(p.args.ViewAccess, p.args.ResourceWeigher, p.args.PricingAccess, p.args.SchedulerLauncher, req)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", service.ErrUnsupportedSimulationStrategy, req.SimulationStrategy)
	}
}

func (p *Planner) doGenerate(ctx context.Context, runArgs *RunArgs, logPath string) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	if err = validateRequest(runArgs.Request); err != nil {
		return
	}
	baseView := p.args.ViewAccess.GetBaseView()
	err = synchronizeBaseView(ctx, baseView, runArgs.Request.Snapshot)
	if err != nil {
		return
	}

	var groupRunPassCounter atomic.Uint32
	groups, err := p.createSimulationGroups(ctx, runArgs, &groupRunPassCounter)
	if err != nil {
		return
	}
	allWinnerNodeScores, unscheduledPods, err := p.runPasses(ctx, runArgs, groups, &groupRunPassCounter, logPath)
	if err != nil {
		return
	}
	if len(allWinnerNodeScores) == 0 {
		log.Info("No scaling advice generated. No winning nodes produced by any simulation group.")
		err = service.ErrNoScalingAdvice
		return
	}
	if runArgs.Request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
		err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassCounter.Load(), allWinnerNodeScores, unscheduledPods, logPath)
	}
	return
}

// runPasses FIXME TODO needs to be refactored into separate Passes abstraction to avoid so many arguments being passed.
func (p *Planner) runPasses(ctx context.Context, runArgs *RunArgs, groups []service.SimulationGroup, groupRunPassCounter *atomic.Uint32, logPath string) (allWinnerNodeScores []service.NodeScore, unscheduledPods []types.NamespacedName, err error) {
	log := logr.FromContextOrDiscard(ctx)
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			log.Info("Planner context done. Aborting pass loop", "err", err)
			return
		default:
			var passWinnerNodeScores []service.NodeScore
			groupRunPassNum := groupRunPassCounter.Load()
			log := log.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
			passCtx := logr.NewContext(ctx, log)
			passWinnerNodeScores, unscheduledPods, err = p.runPass(passCtx, groups)
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
			if runArgs.Request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeIncremental {
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

func validateRequest(request service.ScalingAdviceRequest) error {
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
		return fmt.Errorf("%w: constraint name must not be empty", service.ErrInvalidScalingConstraint)
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		return fmt.Errorf("%w: constraint namespace must not be empty", service.ErrInvalidScalingConstraint)
	}
	return nil
}

func validateClusterSnapshot(cs *service.ClusterSnapshot) error {
	// Check if all nodes have the required label commonconstants.LabelNodeTemplateName
	for _, nodeInfo := range cs.Nodes {
		if _, ok := nodeInfo.Labels[commonconstants.LabelNodeTemplateName]; !ok {
			return fmt.Errorf("%w: node %q has no label %q", service.ErrMissingRequiredLabel, nodeInfo.Name, commonconstants.LabelNodeTemplateName)
		}
	}
	return nil
}

func synchronizeBaseView(ctx context.Context, view minkapi.View, cs *service.ClusterSnapshot) error {
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
func sendScalingAdvice(adviceCh chan<- service.ScalingAdviceResult, request service.ScalingAdviceRequest, groupRunPassNum uint32, winnerNodeScores []service.NodeScore, unscheduledPods []types.NamespacedName, logPath string) error {
	scalingAdvice, err := createScalingAdvice(request, groupRunPassNum, winnerNodeScores, unscheduledPods)
	if err != nil {
		return err
	}
	var msg string
	if request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
		msg = fmt.Sprintf("%s scaling advice for total num passes %d with %d pending unscheduled pods", request.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
	} else {
		msg = fmt.Sprintf("%s scaling advice for pass %d with %d pending unscheduled pods", request.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
	}

	response := service.ScalingAdviceResponse{
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

	adviceCh <- service.ScalingAdviceResult{
		Response: &response,
	}

	return nil
}

func (p *Planner) runPass(ctx context.Context, groups []service.SimulationGroup) (winnerNodeScores []service.NodeScore, unscheduledPods []types.NamespacedName, err error) {
	log := logr.FromContextOrDiscard(ctx)
	var (
		groupRunResult service.SimulationGroupResult
		groupScores    service.SimulationGroupScores
	)
	for _, group := range groups {
		groupRunResult, err = group.Run(ctx)
		if err != nil {
			return
		}
		groupScores, err = computeSimGroupScores(p.args.PricingAccess, p.args.ResourceWeigher, p.args.NodeScorer, p.args.Selector, &groupRunResult)
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

func computeSimGroupScores(pricer service.InstancePricingAccess, weigher service.ResourceWeigher, scorer service.NodeScorer, groupResult *service.SimulationGroupResult) (service.SimulationGroupScores, error) {
	var nodeScores []service.NodeScore
	for _, sr := range groupResult.SimulationResults {
		nodeScore, err := scorer.Compute(sr.NodeScorerArgs)
		if err != nil {
			return service.SimulationGroupScores{}, fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", service.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
		}
		nodeScores = append(nodeScores, nodeScore)
	}
	winnerNodeScore, err := selector(nodeScores, weigher, pricer)
	if err != nil {
		return service.SimulationGroupScores{}, fmt.Errorf("%w: node score selection failed for group %q: %w", service.ErrSelectNodeScore, groupResult.Name, err)
	}
	//if winnerScoreIndex < 0 {
	//	return nil, nil //No winning score for this group
	//}
	//winnerNode := getScaledNodeOfWinner(groupResult.SimulationResults, winnerNodeScore)
	//if winnerNode == nil {
	//	return nil, fmt.Errorf("%w: winner node not found for group %q", api.ErrSelectNodeScore, groupResult.InstanceType)
	//}
	return service.SimulationGroupScores{
		AllNodeScores:   nodeScores,
		WinnerNodeScore: winnerNodeScore,
		//WinnerNode:      winnerNode,
	}, nil
}

//func getScaledNodeOfWinner(simRunResults []service.SimulationResult, winnerNodeScore *service.NodeScore) *corev1.NodeResources {
//	var (
//		winnerNode *corev1.NodeResources
//	)
//	for _, sr := range simRunResults {
//		if sr.Name == winnerNodeScore.Name {
//			winnerNode = sr.ScaledNode
//			break
//		}
//	}
//	return winnerNode
//}

// createSimulationGroups creates a slice of SimulationGroup based on priorities that are defined at the NodePool and NodeTemplate level.
func (p *Planner) createSimulationGroups(ctx context.Context, runArgs *RunArgs, groupRunPassCounter *atomic.Uint32) ([]service.SimulationGroup, error) {
	request := runArgs.Request
	var (
		allSimulations []service.Simulation
	)
	for _, nodePool := range request.Constraint.Spec.NodePools {
		for _, nodeTemplate := range nodePool.NodeTemplates {
			for _, zone := range nodePool.AvailabilityZones {
				simulationName := fmt.Sprintf("%s_%s_%s", nodePool.Name, nodeTemplate.Name, zone)

				sim, err := p.createSimulation(ctx, simulationName, &nodePool, nodeTemplate.Name, zone, groupRunPassCounter)
				if err != nil {
					return nil, err
				}
				allSimulations = append(allSimulations, sim)
			}
		}
	}
	return p.args.SimulationGrouper.Group(allSimulations)
}

func (p *Planner) createSimulation(ctx context.Context, simulationName string, nodePool *sacorev1alpha1.NodePool, nodeTemplateName string, zone string, groupRunPassCounter *atomic.Uint32) (service.Simulation, error) {
	simView, err := p.args.ViewAccess.GetSandboxView(ctx, simulationName)
	if err != nil {
		return nil, err
	}
	simArgs := &service.SimulationArgs{
		RunCounter:        groupRunPassCounter,
		AvailabilityZone:  zone,
		NodePool:          nodePool,
		NodeTemplateName:  nodeTemplateName,
		SchedulerLauncher: p.args.SchedulerLauncher,
		View:              simView,
		TrackPollInterval: 10 * time.Millisecond,
	}
	return p.args.SimulationCreator.Create(simulationName, simArgs)
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
