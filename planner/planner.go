// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"github.com/gardener/scaling-advisor/planner/scorer"
	"github.com/gardener/scaling-advisor/planner/simulation"
	"github.com/gardener/scaling-advisor/planner/simulator/multi"
	"github.com/gardener/scaling-advisor/planner/util"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
)

var _ plannerapi.ScalingPlanner = (*defaultPlanner)(nil)

// defaultPlanner is responsible for creating and managing simulations to generate scaling advice plans.
type defaultPlanner struct {
	args plannerapi.ScalingPlannerArgs
}

// New creates a new instance of defaultPlanner using the provided Args. It initializes the defaultPlanner struct.
func New(args plannerapi.ScalingPlannerArgs) plannerapi.ScalingPlanner {
	return &defaultPlanner{
		args: args,
	}
}

func (p *defaultPlanner) Plan(ctx context.Context, req plannerapi.Request) <-chan plannerapi.Response {
	var err error
	responseCh := make(chan plannerapi.Response)
	go func() {
		defer close(responseCh)
		err = p.doPlan(ctx, &req, responseCh)
		if err != nil {
			util.SendErrorResponse(responseCh, req.GetRef(), err)
		}
	}()
	return responseCh
}

func (p *defaultPlanner) doPlan(ctx context.Context, req *plannerapi.Request, responseCh chan plannerapi.Response) error {
	planCtx, logCloser, err := wrapPlanContext(ctx, p.args.TraceLogsBaseDir, req)
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(logCloser)
	if err = validateRequest(req); err != nil {
		return err
	}
	scaleOutSimulator, err := p.getScaleOutSimulator(req)
	if err != nil {
		return err
	}
	defer ioutil.CloseQuietly(scaleOutSimulator)
	simulationCreator := plannerapi.SimulationCreatorFunc(simulation.NewDefault)
	planResultCh := scaleOutSimulator.Simulate(planCtx, req, simulationCreator)
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
	if !commontypes.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", plannerapi.ErrInvalidRequest, req.AdviceGenerationMode)
	}
	return nil
}

func (p *defaultPlanner) getScaleOutSimulator(req *plannerapi.Request) (plannerapi.ScaleOutSimulator, error) {
	switch req.SimulatorStrategy {
	case "":
		return nil, fmt.Errorf("%w: simulation strategy must be specified", plannerapi.ErrCreateSimulator)
	case commontypes.SimulatorStrategySingleNodeMultiSim:
		nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulator, err)
		}
		return multi.NewScaleOutSimulator(p.args.SimulatorConfig, p.args.ViewAccess, p.args.SchedulerLauncher, nodeScorer)
	case commontypes.SimulatorStrategyMultiNodeSingleSim:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", commonerrors.ErrUnimplemented, req.SimulatorStrategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", plannerapi.ErrUnsupportedSimulatorStrategy, req.SimulatorStrategy)
	}
}

func wrapPlanContext(ctx context.Context, traceLogsDir string, req *plannerapi.Request) (genCtx context.Context, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", req.ID, "correlationID", req.CorrelationID))
	genCtx = context.WithValue(genCtx, commontypes.VerbosityCtxKey, req.DiagnosticVerbosity)
	if req.DiagnosticVerbosity > 0 {
		if traceLogsDir == "" {
			traceLogsDir = ioutil.GetTempDir()
		}
		filepath.Clean(traceLogsDir)
		logPath := path.Join(traceLogsDir, logutil.GetCleanLogFileName(fmt.Sprintf("%s.log", req.ID)))
		genCtx, logCloser, err = logutil.WrapContextWithFileLogger(genCtx, req.CorrelationID, logPath)
		log := logr.FromContextOrDiscard(genCtx)
		log.Info("Diagnostics enabled for this request", "logPath", logPath, "diagnosticVerbosity", req.DiagnosticVerbosity)
	}
	return
}
