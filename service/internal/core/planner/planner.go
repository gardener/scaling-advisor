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

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/service/internal/core/simulator/multi"
	"github.com/gardener/scaling-advisor/service/internal/core/util"
	"github.com/gardener/scaling-advisor/service/scorer"
	"github.com/go-logr/logr"
)

var _ service.ScalingPlanner = (*defaultPlanner)(nil)

// defaultPlanner is responsible for creating and managing simulations to generate scaling advice plans.
type defaultPlanner struct {
	args service.ScalingPlannerArgs
}

//// Args is used to construct a new instance of the defaultPlanner
//type Args struct {
//	ViewAccess        minkapi.ViewAccess
//	PricingAccess     core.InstancePricingAccess
//	ResourceWeigher   core.ResourceWeigher
//	SchedulerLauncher core.SchedulerLauncher
//	TraceLogBaseDir        string
//}

//// RunArgs is used to run the planner and generate scaling advice
//type RunArgs struct {
//	ResultsCh chan<- core.ScalingAdviceResult
//	Request   core.ScalingAdviceRequest
//}

// New creates a new instance of defaultPlanner using the provided Args. It initializes the defaultPlanner struct.
func New(args service.ScalingPlannerArgs) service.ScalingPlanner {
	return &defaultPlanner{
		args: args,
	}
}

func (p *defaultPlanner) Plan(ctx context.Context, req service.ScalingAdviceRequest, resultCh chan<- service.ScalingPlanResult) {
	var err error
	defer func() {
		if err != nil {
			util.SendPlanError(resultCh, req.ScalingAdviceRequestRef, err)
		}
	}()
	planCtx, logCloser, err := wrapPlanContext(ctx, p.args.TraceLogsBaseDir, req)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(logCloser)
	scaleOutSimulator, err := p.getScaleOutSimulator(&req)
	if err != nil {
		return
	}
	scaleOutSimulator.Simulate(planCtx, resultCh)
}

// Run executes the scaling advice generation process using the provided context and runArgs.
// The results of the generation process are sent as one more ScalingAdviceResult's on the RunArgs.ResultsCh channel.
//func (p *defaultPlanner) Run(ctx context.Context, runArgs *RunArgs) {
//	genCtx, logPath, logCloser, err := wrapPlanContext(ctx, p.args.TraceLogBaseDir, runArgs.Request.ID, runArgs.Request.CorrelationID, runArgs.Request.DiagnosticVerbosity)
//	if err != nil {
//		SendPlanError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
//		return
//	}
//	defer ioutil.CloseQuietly(logCloser)
//
//	simulator, err := p.getScaleOutSimulator(&runArgs.Request)
//	if err != nil {
//		SendPlanError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
//		return
//	}
//	planConsumerFn := func(scaleOutPlan sacorev1alpha1.ScaleOutPlan) error {
//		resp := &core.ScalingAdviceResponse{
//			ScalingAdvice: nil,
//			Diagnostics:   nil,
//			RequestRef:    core.ScalingAdviceRequestRef{},
//			Message:       "",
//		}
//		runArgs.ResultsCh <- core.ScalingAdviceResult{
//			Response: resp,
//		}
//		return nil
//	}
//	simulator.Simulate(ctx, planConsumerFn)
//	err = p.doGenerate(genCtx, runArgs, logPath)
//	if err != nil {
//		SendPlanError(runArgs.ResultsCh, runArgs.Request.ScalingAdviceRequestRef, err)
//		return
//	}
//}

func (p *defaultPlanner) getScaleOutSimulator(req *service.ScalingAdviceRequest) (service.ScaleOutSimulator, error) {
	switch req.SimulationStrategy {
	case "":
		return nil, fmt.Errorf("%w: simulation strategy must be specified", service.ErrCreateSimulator)
	case commontypes.SimulationStrategyMultiSimulationsPerGroup:
		nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", service.ErrCreateSimulator, err)
		}
		return multi.NewScaleOutSimulator(p.args.ViewAccess, p.args.SchedulerLauncher, nodeScorer, p.args.SimulatorConfig, req)
	case commontypes.SimulationStrategySingleSimulationPerGroup:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", commonerrors.ErrUnimplemented, req.SimulationStrategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", service.ErrUnsupportedSimulationStrategy, req.SimulationStrategy)
	}
}

//func (p *defaultPlanner) doGenerate(ctx context.Context, runArgs *RunArgs, logPath string) (err error) {
//	log := logr.FromContextOrDiscard(ctx)
//	if err = validateRequest(runArgs.Request); err != nil {
//		return
//	}
//	baseView := p.args.ViewAccess.GetBaseView()
//	err = synchronizeBaseView(ctx, baseView, runArgs.Request.Snapshot)
//	if err != nil {
//		return
//	}
//
//	var groupRunPassCounter atomic.Uint32
//	groups, err := p.createSimulationGroups(ctx, runArgs, &groupRunPassCounter)
//	if err != nil {
//		return
//	}
//	allWinnerNodeScores, unscheduledPods, err := p.runPasses(ctx, runArgs, groups, &groupRunPassCounter, logPath)
//	if err != nil {
//		return
//	}
//	if len(allWinnerNodeScores) == 0 {
//		log.Info("No scaling advice generated. No winning nodes produced by any simulation group.")
//		err = core.ErrNoScalingAdvice
//		return
//	}
//	if runArgs.Request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
//		err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassCounter.Load(), allWinnerNodeScores, unscheduledPods, logPath)
//	}
//	return
//}

// runPasses FIXME TODO needs to be refactored into separate Passes abstraction to avoid so many arguments being passed.
//func (p *defaultPlanner) runPasses(ctx context.Context, runArgs *RunArgs, groups []core.SimulationGroup, groupRunPassCounter *atomic.Uint32, logPath string) (allWinnerNodeScores []core.NodeScore, unscheduledPods []types.NamespacedName, err error) {
//	log := logr.FromContextOrDiscard(ctx)
//	for {
//		select {
//		case <-ctx.Done():
//			err = ctx.Err()
//			log.Info("defaultPlanner context done. Aborting pass loop", "err", err)
//			return
//		default:
//			var passWinnerNodeScores []core.NodeScore
//			groupRunPassNum := groupRunPassCounter.Load()
//			log := log.WithValues("groupRunPass", groupRunPassNum) // purposefully shadowed.
//			passCtx := logr.NewContext(ctx, log)
//			passWinnerNodeScores, unscheduledPods, err = p.runPass(passCtx, groups)
//			if err != nil {
//				return
//			}
//			// If there are no winning nodes produced by a pass for the pending unscheduled pods, then abort the loop.
//			// This means that we could not identify any node from the node pool and node template combinations (as specified in the constraint)
//			// that could accommodate any unscheduled pods. It is fruitless to continue further.
//			if len(passWinnerNodeScores) == 0 {
//				log.Info("Aborting loop since no node scores produced in pass.", "groupRunPass", groupRunPassNum)
//				return
//			}
//			allWinnerNodeScores = append(allWinnerNodeScores, passWinnerNodeScores...)
//			if runArgs.Request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeIncremental {
//				err = sendScalingAdvice(runArgs.ResultsCh, runArgs.Request, groupRunPassNum, passWinnerNodeScores, unscheduledPods, logPath)
//				if err != nil {
//					return
//				}
//			}
//			if len(unscheduledPods) == 0 {
//				log.Info("All pods have been scheduled in pass", "groupRunPass", groupRunPassNum)
//				return
//			}
//			groupRunPassCounter.Add(1)
//		}
//	}
//}

//func validateRequest(request core.ScalingAdviceRequest) error {
//	if err := validateConstraint(request.Constraint); err != nil {
//		return err
//	}
//	if err := validateClusterSnapshot(request.Snapshot); err != nil {
//		return err
//	}
//	return nil
//}

//func validateConstraint(constraint *sacorev1alpha1.ClusterScalingConstraint) error {
//	if strings.TrimSpace(constraint.Name) == "" {
//		return fmt.Errorf("%w: constraint name must not be empty", core.ErrInvalidScalingConstraint)
//	}
//	if strings.TrimSpace(constraint.Namespace) == "" {
//		return fmt.Errorf("%w: constraint namespace must not be empty", core.ErrInvalidScalingConstraint)
//	}
//	return nil
//}

//func validateClusterSnapshot(cs *core.ClusterSnapshot) error {
//	// Check if all nodes have the required label commonconstants.LabelNodeTemplateName
//	for _, nodeInfo := range cs.Nodes {
//		if _, ok := nodeInfo.Labels[commonconstants.LabelNodeTemplateName]; !ok {
//			return fmt.Errorf("%w: node %q has no label %q", core.ErrMissingRequiredLabel, nodeInfo.Name, commonconstants.LabelNodeTemplateName)
//		}
//	}
//	return nil
//}

//func synchronizeBaseView(ctx context.Context, view minkapi.View, cs *core.ClusterSnapshot) error {
//	// TODO implement delta cluster snapshot to update the base view before every simulation run which will synchronize
//	// the base view with the current state of the target cluster.
//	view.Reset()
//	for _, nodeInfo := range cs.Nodes {
//		if _, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo)); err != nil {
//			return err
//		}
//	}
//	for _, pod := range cs.Pods {
//		if _, err := view.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod)); err != nil {
//			return err
//		}
//	}
//	for _, pc := range cs.PriorityClasses {
//		if _, err := view.CreateObject(ctx, typeinfo.PriorityClassesDescriptor.GVK, &pc); err != nil {
//			return err
//		}
//	}
//	for _, rc := range cs.RuntimeClasses {
//		if _, err := view.CreateObject(ctx, typeinfo.RuntimeClassDescriptor.GVK, &rc); err != nil {
//			return err
//		}
//	}
//	return nil
//}

// sendScalingAdvice needs to be fixed: FIXME, TODO: reduce num of args by making this a method or wrapping args into struct or alternative
//func sendScalingAdvice(adviceCh chan<- core.ScalingAdviceResult, request core.ScalingAdviceRequest, groupRunPassNum uint32, winnerNodeScores []core.NodeScore, unscheduledPods []types.NamespacedName, logPath string) error {
//	scalingAdvice, err := createScalingAdvice(request, groupRunPassNum, winnerNodeScores, unscheduledPods)
//	if err != nil {
//		return err
//	}
//	var msg string
//	if request.AdviceGenerationMode == commontypes.ScalingAdviceGenerationModeAllAtOnce {
//		msg = fmt.Sprintf("%s scaling advice for total num passes %d with %d pending unscheduled pods", request.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
//	} else {
//		msg = fmt.Sprintf("%s scaling advice for pass %d with %d pending unscheduled pods", request.AdviceGenerationMode, groupRunPassNum, len(unscheduledPods))
//	}
//
//	response := core.ScalingAdviceResponse{
//		RequestRef:    request.ScalingAdviceRequestRef,
//		Message:       msg,
//		ScalingAdvice: scalingAdvice,
//	}
//
//	if request.DiagnosticVerbosity {
//		response.Diagnostics = &sacorev1alpha1.ScalingAdviceDiagnostic{
//			SimRunResults: nil, // TODO: populate SimRunResults
//			TraceLogName:  logPath,
//		}
//	}
//
//	adviceCh <- core.ScalingAdviceResult{
//		Response: &response,
//	}
//
//	return nil
//}

//func (p *defaultPlanner) runPass(ctx context.Context, groups []core.SimulationGroup) (winnerNodeScores []core.NodeScore, unscheduledPods []types.NamespacedName, err error) {
//	log := logr.FromContextOrDiscard(ctx)
//	var (
//		groupRunResult core.SimulationGroupResult
//		groupScores    core.SimulationGroupScores
//	)
//	for _, group := range groups {
//		groupRunResult, err = group.Run(ctx)
//		if err != nil {
//			return
//		}
//		groupScores, err = computeSimGroupScores(p.args.PricingAccess, p.args.ResourceWeigher, p.args.NodeScorer, p.args.Selector, &groupRunResult)
//		if err != nil {
//			return
//		}
//		if groupScores.WinnerNodeScore == nil {
//			log.Info("simulation group did not produce any winning score. Skipping this group.", "simulationGroupName", groupRunResult.Name)
//			continue
//		}
//		winnerNodeScores = append(winnerNodeScores, *groupScores.WinnerNodeScore)
//		unscheduledPods = groupScores.WinnerNodeScore.UnscheduledPods
//		if len(groupScores.WinnerNodeScore.UnscheduledPods) == 0 {
//			log.Info("simulation group winner has left NO unscheduled pods. No need to continue to next group", "simulationGroupName", groupRunResult.Name)
//			break
//		}
//	}
//	return
//}

//func computeSimGroupScores(pricer core.InstancePricingAccess, weigher core.ResourceWeigher, scorer core.NodeScorer, groupResult *core.SimulationGroupResult) (core.SimulationGroupScores, error) {
//	var nodeScores []core.NodeScore
//	for _, sr := range groupResult.SimulationResults {
//		nodeScore, err := scorer.Compute(sr.NodeScorerArgs)
//		if err != nil {
//			return core.SimulationGroupScores{}, fmt.Errorf("%w: node scoring failed for simulation %q of group %q: %w", core.ErrComputeNodeScore, sr.Name, groupResult.Name, err)
//		}
//		nodeScores = append(nodeScores, nodeScore)
//	}
//	winnerNodeScore, err := selector(nodeScores, weigher, pricer)
//	if err != nil {
//		return core.SimulationGroupScores{}, fmt.Errorf("%w: node score selection failed for group %q: %w", core.ErrSelectNodeScore, groupResult.Name, err)
//	}
//	//if winnerScoreIndex < 0 {
//	//	return nil, nil //No winning score for this group
//	//}
//	//winnerNode := getScaledNodeOfWinner(groupResult.SimulationResults, winnerNodeScore)
//	//if winnerNode == nil {
//	//	return nil, fmt.Errorf("%w: winner node not found for group %q", api.ErrSelectNodeScore, groupResult.InstanceType)
//	//}
//	return core.SimulationGroupScores{
//		AllNodeScores:   nodeScores,
//		WinnerNodeScore: winnerNodeScore,
//		//WinnerNode:      winnerNode,
//	}, nil
//}

//func getScaledNodeOfWinner(simRunResults []core.SimulationResult, winnerNodeScore *core.NodeScore) *corev1.NodeResources {
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

func wrapPlanContext(ctx context.Context, traceLogsDir string, req service.ScalingAdviceRequest) (genCtx context.Context, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", req.ID, "correlationID", req.CorrelationID))
	genCtx = context.WithValue(genCtx, commonconstants.VerbosityCtxKey, req.DiagnosticVerbosity)
	if req.DiagnosticVerbosity > 0 {
		if traceLogsDir == "" {
			traceLogsDir = os.TempDir()
		}
		logPath := path.Join(traceLogsDir, fmt.Sprintf("%s-%s.log", req.CorrelationID, req.ID))
		genCtx, logCloser, err = logutil.WrapContextWithFileLogger(genCtx, req.CorrelationID, logPath)
		log := logr.FromContextOrDiscard(genCtx)
		log.Info("Diagnostics enabled for this request", "logPath", logPath)
	}
	return
}
