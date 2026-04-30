// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/gardener/scaling-advisor/planner/scorer"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScalingPlanner = (*defaultPlanner)(nil)
)

// defaultPlanner is responsible for creating and managing simulations to generate scaling advice plans.
type defaultPlanner struct {
	args planner.ScalingPlannerArgs
}

// NewPlanner creates a new instance of the default ScalingPlanner using the provided Args.
func NewPlanner(args plannerapi.ScalingPlannerArgs) (plannerapi.ScalingPlanner, error) {
	if err := validateArgs(&args); err != nil {
		return nil, err
	}
	return &defaultPlanner{
		args: args,
	}, nil
}

func (p *defaultPlanner) Plan(ctx context.Context, req planner.ScalingAdviceRequest, resultCh chan<- planner.ScalingPlanResult) {
	var err error
	defer func() {
		if err != nil {
			SendErrorResponse(responseCh, req.GetRef(), err)
		}
	}()
	return responseCh
}

// Do we need to send back Memento?
func (p *defaultPlanner) doPlan(ctx context.Context, req *plannerapi.Request, responseCh chan plannerapi.Response) error {
	planCtx, logCloser, err := wrapPlanContext(ctx, p.args.TraceDir, req)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(logCloser)
	if err = validateRequest(req); err != nil {
		return
	}
	scaleInSimulator, err := p.args.SimulatorFactory.GetScaleInSimulator(plannerapi.SimulatorArgs{
		ScaleInCandidateSelector: p.args.ScaleInCandidateSelector,
		ScaleInSimulatorConfig:   p.args.ScaleInSimulatorConfig,
		ViewAccess:               p.args.ViewAccess,
		SchedulerLauncher:        p.args.SchedulerLauncher,
		TraceDir:                 p.args.TraceDir,
	})
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(scaleInSimulator)
	scaleInPlanResultCh := scaleInSimulator.Simulate(planCtx, req, p.args.SimulationFactory)
	nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulator, err)
	}
	scaleOutSimulator, err := p.args.SimulatorFactory.GetScaleOutSimulator(plannerapi.SimulatorArgs{
		Config:            p.args.SimulatorConfig,
		Strategy:          req.SimulatorStrategy,
		ViewAccess:        p.args.ViewAccess,
		SchedulerLauncher: p.args.SchedulerLauncher,
		StorageMetaAccess: p.args.StorageMetaAccess,
		NodeScorer:        nodeScorer,
		TraceDir:          p.args.TraceDir,
	})
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(scaleOutSimulator)
	scaleOutPlanResultCh := scaleOutSimulator.Simulate(planCtx, req, p.args.SimulationFactory)
	var scaleInPlanResult plannerapi.ScaleInPlanResult
	var scaleOutPlanResult plannerapi.ScaleOutPlanResult
	for receivedCount := 0; receivedCount < 2; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r, ok := <-scaleInPlanResultCh:
			if ok {
				scaleInPlanResult = r
				scaleInPlanResultCh = nil
				receivedCount++
			}
		case r, ok := <-scaleOutPlanResultCh:
			if ok {
				scaleOutPlanResult = r
				scaleOutPlanResultCh = nil
				receivedCount++
			}
		}
	}
	// TODO: what do I send in response.Labels? -> union of scalein and scaleout plan maybe extract the common part
	response := plannerapi.Response{
		RequestRef:   req.RequestRef,
		ID:           objutil.GenerateName("scaling-plan-"),
		ScaleInPlan:  scaleInPlanResult.ScaleInPlan,
		ScaleOutPlan: scaleOutPlanResult.ScaleOutPlan,
	}
	if scaleInPlanResult.Error != nil && !errors.Is(scaleInPlanResult.Error, plannerapi.ErrNoScaleInPlan) {
		response.Error = scaleInPlanResult.Error
		responseCh <- response
		return nil
	} else if scaleOutPlanResult.Error != nil && !errors.Is(scaleOutPlanResult.Error, plannerapi.ErrNoScaleOutPlan) {
		response.Error = scaleOutPlanResult.Error
		responseCh <- response
		return nil
	}
	composeScaleOutAndScaleInPlanItems(response.ScaleOutPlan, response.ScaleInPlan)
	responseCh <- response
	return nil
}

func validateRequest(req planner.ScalingAdviceRequest) error {
	if !commontypes.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", planner.ErrInvalidScalingAdviceRequest, req.AdviceGenerationMode)
	}
	return nil
}

func wrapPlanContext(ctx context.Context, traceDir string, req *plannerapi.Request) (genCtx context.Context, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", req.ID, "correlationID", req.CorrelationID))
	genCtx = context.WithValue(genCtx, commontypes.VerbosityCtxKey, req.DiagnosticVerbosity)
	if req.DiagnosticVerbosity > 1 {
		if traceDir == "" {
			traceDir = ioutil.GetTempDir()
		}
		genCtx = context.WithValue(genCtx, commontypes.TraceDirCtxKey, traceDir)
		genCtx = context.WithValue(genCtx, commontypes.VerbosityCtxKey, req.DiagnosticVerbosity)
		filepath.Clean(traceDir)
		logPath := path.Join(traceDir, logutil.GetCleanLogFileName(fmt.Sprintf("%s.log", req.ID)))
		genCtx, logCloser, err = logutil.WrapContextWithFileLogger(genCtx, req.CorrelationID, logPath)
		log := logr.FromContextOrDiscard(genCtx)
		log.Info("Diagnostics enabled for this request", "logPath", logPath)
	}
	return
}

func validateArgs(args *plannerapi.ScalingPlannerArgs) error {
	if args.ResourceWeigher == nil {
		return fmt.Errorf("%w: resourceWeigher must be set", plannerapi.ErrCreatePlanner)
	}
	if args.ViewAccess == nil {
		return fmt.Errorf("%w: viewAccess must be set", plannerapi.ErrCreatePlanner)
	}
	if args.PricingAccess == nil {
		return fmt.Errorf("%w: pricingAccess must be set", plannerapi.ErrCreatePlanner)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("%w: schedulerLauncher must be set", plannerapi.ErrCreatePlanner)
	}
	if args.StorageMetaAccess == nil {
		return fmt.Errorf("%w: storageMetaAccess must be set", plannerapi.ErrCreatePlanner)
	}
	if args.SimulatorFactory == nil {
		return fmt.Errorf("%w: simulatorFactory must be set", plannerapi.ErrCreatePlanner)
	}
	if args.SimulationFactory == nil {
		return fmt.Errorf("%w: simulationFactory must be set", plannerapi.ErrCreatePlanner)
	}
	return nil
}

// SendErrorResponse wraps the given error with the sentinel error plannerapi.ErrGenScalingPlan, embeds the wrapped error
// within a plannerapi.Response and sends the response to the given results channel.
func SendErrorResponse(resultsCh chan<- plannerapi.Response, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- plannerapi.Response{
		ID:    objutil.GenerateName("plan-error"),
		Error: err,
	}
}

// composeScaleOutAndScaleInPlanItems removes redundant operations where the same
// NodePlacement appears in both plans. For each overlapping placement the
// smaller count is subtracted from both sides so we never scale-out and
// scale-in the same placement.
func composeScaleOutAndScaleInPlanItems(scaleOutPlan *sacorev1alpha1.ScaleOutPlan, scaleInPlan *sacorev1alpha1.ScaleInPlan) {
	if scaleOutPlan == nil || scaleInPlan == nil {
		return
	}
	// Count how many nodes are being scaled in per NodePlacement.
	scaleInCountByPlacement := make(map[sacorev1alpha1.NodePlacement]int32)
	for _, item := range scaleInPlan.Items {
		scaleInCountByPlacement[item.NodePlacement]++
	}
	// Build a map of scale-out deltas by NodePlacement.
	scaleOutDeltaByPlacement := make(map[sacorev1alpha1.NodePlacement]int32)
	for _, item := range scaleOutPlan.Items {
		scaleOutDeltaByPlacement[item.NodePlacement] += item.Delta
	}
	// Compute the overlap: for each placement, the number of redundant nodes is min(scaleIn, scaleOut).
	overlapByPlacement := make(map[sacorev1alpha1.NodePlacement]int32)
	for placement, scaleInCount := range scaleInCountByPlacement {
		if scaleOutDelta, ok := scaleOutDeltaByPlacement[placement]; ok {
			overlapByPlacement[placement] = min(scaleInCount, scaleOutDelta)
		}
	}
	if len(overlapByPlacement) == 0 {
		return
	}
	// Remove overlap from scale-out items.
	composedScaleOutItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(scaleOutPlan.Items))
	for _, item := range scaleOutPlan.Items {
		if overlap := overlapByPlacement[item.NodePlacement]; overlap > 0 {
			reduction := min(overlap, item.Delta)
			overlapByPlacement[item.NodePlacement] -= reduction
			item.Delta -= reduction
			if item.Delta <= 0 {
				continue
			}
		}
		composedScaleOutItems = append(composedScaleOutItems, item)
	}
	scaleOutPlan.Items = composedScaleOutItems
	// Re-compute overlap for scale-in removal (reset from original counts).
	for placement := range overlapByPlacement {
		scaleInCount := scaleInCountByPlacement[placement]
		scaleOutDelta := scaleOutDeltaByPlacement[placement]
		overlapByPlacement[placement] = min(scaleInCount, scaleOutDelta)
	}
	// Remove overlap from scale-in items.
	composedScaleInItems := make([]sacorev1alpha1.ScaleInItem, 0, len(scaleInPlan.Items))
	for _, item := range scaleInPlan.Items {
		if overlap := overlapByPlacement[item.NodePlacement]; overlap > 0 {
			overlapByPlacement[item.NodePlacement]--
			continue
		}
		composedScaleInItems = append(composedScaleInItems, item)
	}
	scaleInPlan.Items = composedScaleInItems
}
