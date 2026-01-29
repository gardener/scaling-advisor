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
	"github.com/gardener/scaling-advisor/planner/simulator/multi"
	"github.com/gardener/scaling-advisor/planner/util"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/go-logr/logr"
)

var _ planner.ScalingPlanner = (*defaultPlanner)(nil)

// defaultPlanner is responsible for creating and managing simulations to generate scaling advice plans.
type defaultPlanner struct {
	args planner.ScalingPlannerArgs
}

// New creates a new instance of defaultPlanner using the provided Args. It initializes the defaultPlanner struct.
func New(args planner.ScalingPlannerArgs) planner.ScalingPlanner {
	return &defaultPlanner{
		args: args,
	}
}

func (p *defaultPlanner) Plan(ctx context.Context, req planner.ScalingAdviceRequest, resultCh chan<- planner.ScalingPlanResult) {
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
	if err = validateRequest(req); err != nil {
		return
	}
	scaleOutSimulator, err := p.getScaleOutSimulator(&req)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(scaleOutSimulator)
	scaleOutSimulator.Simulate(planCtx, resultCh)
}

func validateRequest(req planner.ScalingAdviceRequest) error {
	if req.CreationTime.IsZero() {
		return fmt.Errorf("%w: createdTime not set", planner.ErrInvalidScalingAdviceRequest)
	}
	if !commontypes.SupportedAdviceGenerationModes.Has(req.AdviceGenerationMode) {
		return fmt.Errorf("%w: unsupported advice generation mode %q", planner.ErrInvalidScalingAdviceRequest, req.AdviceGenerationMode)
	}
	return nil
}

func (p *defaultPlanner) getScaleOutSimulator(req *planner.ScalingAdviceRequest) (planner.ScaleOutSimulator, error) {
	switch req.SimulationStrategy {
	case "":
		return nil, fmt.Errorf("%w: simulation strategy must be specified", planner.ErrCreateSimulator)
	case commontypes.SimulationStrategyMultiSimulationsPerGroup:
		nodeScorer, err := scorer.GetNodeScorer(req.ScoringStrategy, p.args.PricingAccess, p.args.ResourceWeigher)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", planner.ErrCreateSimulator, err)
		}
		return multi.NewScaleOutSimulator(p.args.ViewAccess, p.args.SchedulerLauncher, nodeScorer, p.args.SimulatorConfig, req)
	case commontypes.SimulationStrategySingleSimulationPerGroup:
		return nil, fmt.Errorf("%w: simulation strategy %q not yet implemented", commonerrors.ErrUnimplemented, req.SimulationStrategy)
	default:
		return nil, fmt.Errorf("%w: unsupported simulation strategy %q", planner.ErrUnsupportedSimulationStrategy, req.SimulationStrategy)
	}
}

func wrapPlanContext(ctx context.Context, traceLogsDir string, req planner.ScalingAdviceRequest) (genCtx context.Context, logCloser io.Closer, err error) {
	genCtx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", req.ID, "correlationID", req.CorrelationID))
	genCtx = context.WithValue(genCtx, commonconstants.VerbosityCtxKey, req.DiagnosticVerbosity)
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
