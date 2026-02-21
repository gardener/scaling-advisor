// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/planner/scorer"
	"github.com/gardener/scaling-advisor/planner/util"
	"io"
	"os"
	"path"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
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
			util.SendPlanError(resultCh, req.ScalingAdviceRequestRef, err)
		}
	}()
	return responseCh
}

func (p *defaultPlanner) doPlan(ctx context.Context, req *plannerapi.Request, responseCh chan plannerapi.Response) error {
	planCtx, logCloser, err := wrapPlanContext(ctx, p.args.TraceDir, req)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(logCloser)
	if err = validateRequest(req); err != nil {
		return
	}
	nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
	if err != nil {
		err = fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulator, err)
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
	planResultCh := scaleOutSimulator.Simulate(planCtx, req, p.args.SimulationFactory)
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
	scaleOutSimulator.Simulate(planCtx, resultCh)
}

func validateRequest(req planner.ScalingAdviceRequest) error {
	if !commontypes.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", planner.ErrInvalidScalingAdviceRequest, req.AdviceGenerationMode)
	}
	return nil
}

func wrapPlanContext(ctx context.Context, traceLogsDir string, req *plannerapi.Request) (genCtx context.Context, logCloser io.Closer, err error) {
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
