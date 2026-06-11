// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"

	"github.com/gardener/scaling-advisor/planner/scorer"
	"github.com/gardener/scaling-advisor/planner/simulator"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScalingPlanner = (*defaultPlanner)(nil)
)

// defaultPlanner is responsible for creating and managing simulations to generate scaling advice plans.
type defaultPlanner struct {
	args plannerapi.ScalingPlannerArgs
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

func (p *defaultPlanner) Plan(ctx context.Context, req plannerapi.Request) <-chan plannerapi.Response {
	var err error
	responseCh := make(chan plannerapi.Response)
	go func() {
		defer close(responseCh)
		err = p.doPlan(ctx, &req, responseCh)
		if err != nil {
			SendErrorResponse(responseCh, req.GetRef(), err)
		}
	}()
	return responseCh
}

func (p *defaultPlanner) doPlan(ctx context.Context, req *plannerapi.Request, responseCh chan plannerapi.Response) error {
	planCtx, logCloser, err := wrapPlanContext(ctx, p.args.TraceDir, req)
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(logCloser)
	if err = validateRequest(req); err != nil {
		return err
	}
	requestView, err := simulator.InitializeRequestView(planCtx, req, p.args.ViewAccess, p.args.SimulatorConfig)
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(requestView)

	response := plannerapi.Response{
		RequestRef: req.RequestRef,
		ID:         objutil.GenerateName("scaling-plan-"),
		Labels:     make(map[string]string),
	}

	scaleInResult, err := p.doScaleIn(planCtx, req, requestView)
	if err != nil {
		return err
	}
	maps.Copy(response.Labels, scaleInResult.Labels)
	response.ScaleInPlan = scaleInResult.ScaleInPlan

	// Use the post-scale-in view as the starting point for scale-out.
	// If scale-in produced no usable view, fall back to the original request view.
	scaleOutView := scaleInResult.View
	if scaleOutView == nil {
		scaleOutView = requestView
	}

	scaleOutResult, err := p.doScaleOut(planCtx, req, scaleOutView)
	if err != nil {
		return err
	}
	maps.Copy(response.Labels, scaleOutResult.Labels)
	response.ScaleOutPlan = scaleOutResult.ScaleOutPlan

	responseCh <- response
	return nil
}

func (p *defaultPlanner) doScaleIn(ctx context.Context, req *plannerapi.Request, requestView minkapi.View) (plannerapi.ScaleInPlanResult, error) {
	scaleInSimulator, err := p.args.SimulatorFactory.GetScaleInSimulator(plannerapi.SimulatorArgs{
		ScaleInCandidateSelector: p.args.ScaleInCandidateSelector,
		Config:                   p.args.SimulatorConfig,
		ViewAccess:               p.args.ViewAccess,
		SchedulerLauncher:        p.args.SchedulerLauncher,
		TraceDir:                 p.args.TraceDir,
		SimulationFactory:        p.args.SimulationFactory,
	})
	if err != nil {
		return plannerapi.ScaleInPlanResult{}, err
	}
	defer ioutil.CloseQuietly(scaleInSimulator)

	select {
	case <-ctx.Done():
		return plannerapi.ScaleInPlanResult{}, ctx.Err()
	case r, ok := <-scaleInSimulator.Simulate(ctx, req, requestView):
		if !ok {
			return plannerapi.ScaleInPlanResult{}, nil
		}
		if r.Error != nil && !errors.Is(r.Error, plannerapi.ErrNoScaleInPlan) {
			return plannerapi.ScaleInPlanResult{}, r.Error
		}
		return r, nil
	}
}

func (p *defaultPlanner) doScaleOut(ctx context.Context, req *plannerapi.Request, requestView minkapi.View) (plannerapi.ScaleOutPlanResult, error) {
	nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
	if err != nil {
		return plannerapi.ScaleOutPlanResult{}, fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulator, err)
	}
	scaleOutSimulator, err := p.args.SimulatorFactory.GetScaleOutSimulator(plannerapi.SimulatorArgs{
		Config:            p.args.SimulatorConfig,
		Strategy:          req.SimulatorStrategy,
		ViewAccess:        p.args.ViewAccess,
		SchedulerLauncher: p.args.SchedulerLauncher,
		StorageMetaAccess: p.args.StorageMetaAccess,
		NodeScorer:        nodeScorer,
		TraceDir:          p.args.TraceDir,
		SimulationFactory: p.args.SimulationFactory,
	})
	if err != nil {
		return plannerapi.ScaleOutPlanResult{}, err
	}
	defer ioutil.CloseQuietly(scaleOutSimulator)

	select {
	case <-ctx.Done():
		return plannerapi.ScaleOutPlanResult{}, ctx.Err()
	case r, ok := <-scaleOutSimulator.Simulate(ctx, req, requestView):
		if !ok {
			return plannerapi.ScaleOutPlanResult{}, nil
		}
		if r.Error != nil && !errors.Is(r.Error, plannerapi.ErrNoScaleOutPlan) {
			return plannerapi.ScaleOutPlanResult{}, r.Error
		}
		return r, nil
	}
}

func validateRequest(req *plannerapi.Request) error {
	if req.CreationTime.IsZero() {
		return fmt.Errorf("%w: createdTime not set", plannerapi.ErrInvalidRequest)
	}
	if !commontypes.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", plannerapi.ErrInvalidRequest, req.AdviceGenerationMode)
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
		log.Info("Diagnostics enabled for this request", "logPath", logPath, "diagnosticVerbosity", req.DiagnosticVerbosity)
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
