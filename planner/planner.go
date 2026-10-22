// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/gardener/scaling-advisor/planner/scorer"
	"github.com/gardener/scaling-advisor/planner/simulator"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/viewutil"
	simvolume "github.com/gardener/scaling-advisor/simulation/simvolume"
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
	nodeScorer, err := scorer.GetNodeScorer(req.ScalingConstraint.ScoringStrategy)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulator, err)
	}
	simArgs := plannerapi.SimulatorArgs{
		Config:            p.args.SimulatorConfig,
		Strategy:          req.ScalingConstraint.SimulatorStrategy,
		ViewAccess:        p.args.ViewAccess,
		SchedulerLauncher: p.args.SchedulerLauncher,
		NodeScorer:        nodeScorer,
		TraceDir:          p.args.TraceDir,
		Request:           req,
	}
	scaleOutSimulator, err := p.args.SimulatorFactory.GetScaleOutSimulator(simArgs)
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(scaleOutSimulator)

	// Build and populate the request view, then pass it to Simulate.
	requestView, err := p.args.ViewAccess.GetSandboxViewOverDelegate(planCtx, "Request-"+req.ID, p.args.ViewAccess.GetBaseView())
	if err != nil {
		return fmt.Errorf("%w: failed to create request view: %w", plannerapi.ErrPopulateRequestView, err)
	}
	if err = simulator.PopulateView(planCtx, requestView, &req.ClusterSnapshot); err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
	}
	if p.args.SimulatorConfig.BindVolumeClaimsForImmediateMode {
		if _, err = simvolume.BindVolumeClaimsForImmediateMode(planCtx, requestView); err != nil {
			return fmt.Errorf("%w: failed to bind volume claims for immediate mode: %w", plannerapi.ErrPopulateRequestView, err)
		}
	}
	if err = viewutil.LogObjects(planCtx, "requestView", requestView); err != nil {
		logr.FromContextOrDiscard(planCtx).Info("failed to dump requestView objects", "error", err)
	}

	planResultCh := scaleOutSimulator.Simulate(planCtx, p.args.SimulationFactory, requestView)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case planResult, ok := <-planResultCh:
			if !ok {
				return nil // planResultCh closed by ScaleOutSimulator.Simulate
			}
			response := plannerapi.Response{
				RequestRef:   req.RequestRef,
				Error:        planResult.Error,
				Labels:       planResult.Labels,
				ScaleOutPlan: planResult.ScaleOutPlan,
				ScaleInPlan:  nil,
				ID:           objutil.GenerateName("scaling-plan-"),
			}
			responseCh <- response
		}
	}
}

func validateRequest(req *plannerapi.Request) error {
	if req.CreationTime.IsZero() {
		return fmt.Errorf("%w: createdTime not set", plannerapi.ErrInvalidRequest)
	}
	if !apicommon.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", plannerapi.ErrInvalidRequest, req.AdviceGenerationMode)
	}
	return nil
}

func wrapPlanContext(ctx context.Context, traceDir string, req *plannerapi.Request) (genCtx context.Context, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", req.ID, "correlationID", req.CorrelationID))
	genCtx = context.WithValue(genCtx, apicommon.VerbosityCtxKey, req.DiagnosticVerbosity)
	if req.DiagnosticVerbosity > 1 {
		if traceDir == "" {
			traceDir = ioutil.GetTempDir()
		}
		genCtx = context.WithValue(genCtx, apicommon.TraceDirCtxKey, traceDir)
		genCtx = context.WithValue(genCtx, apicommon.VerbosityCtxKey, req.DiagnosticVerbosity)
		filepath.Clean(traceDir)
		logPath := path.Join(traceDir, logutil.GetCleanLogFileName(fmt.Sprintf("%s.log", req.ID)))
		genCtx, logCloser, err = logutil.WrapContextWithFileLogger(genCtx, req.CorrelationID, logPath)
		log := logr.FromContextOrDiscard(genCtx)
		log.Info("Diagnostics enabled for this request", "logPath", logPath, "diagnosticVerbosity", req.DiagnosticVerbosity)
	}
	return
}

func validateArgs(args *plannerapi.ScalingPlannerArgs) error {
	if args.ViewAccess == nil {
		return fmt.Errorf("%w: viewAccess must be set", plannerapi.ErrCreatePlanner)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("%w: schedulerLauncher must be set", plannerapi.ErrCreatePlanner)
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
