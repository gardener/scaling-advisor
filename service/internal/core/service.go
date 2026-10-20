// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"

	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	mkcore "github.com/gardener/scaling-advisor/minkapi/server"
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/gardener/scaling-advisor/samples"
	"github.com/gardener/scaling-advisor/simulation/scheduler"
)

var _ plannerapi.ScalingPlannerService = (*defaultPlannerService)(nil)

type defaultPlannerService struct {
	minKAPIServer minkapi.Server
	planner       plannerapi.ScalingPlanner
	cfg           plannerapi.ScalingPlannerServiceConfig
}

// NewService initializes and returns a ScalingPlannerService based on the provided configuration and factories.
func NewService(ctx context.Context,
	config plannerapi.ScalingPlannerServiceConfig,
	factories plannerapi.Factories) (svc plannerapi.ScalingPlannerService, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrServiceInitFailed, err)
		}
	}()
	minKAPIServer, err := mkcore.New(ctx, config.MinKAPIConfig)
	if err != nil {
		return
	}
	schedulerConfigBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		return
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, config.SimulatorConfig.MaxParallelSimulations)
	if err != nil {
		return
	}
	p, err := factories.Planner.NewPlanner(plannerapi.ScalingPlannerArgs{
		ViewAccess:        minKAPIServer,
		SchedulerLauncher: schedulerLauncher,
		SimulatorFactory:  factories.Simulator,
		SimulationFactory: factories.Simulation,
		TraceDir:          config.TraceDir,
		SimulatorConfig:   config.SimulatorConfig,
	})
	if err != nil {
		return
	}
	svc = &defaultPlannerService{
		cfg:           config,
		minKAPIServer: minKAPIServer,
		planner:       p,
	}
	return
}

func (d *defaultPlannerService) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrStartFailed, err)
		}
	}()
	if err = d.minKAPIServer.Start(ctx); err != nil {
		return
	}
	return
}

func (d *defaultPlannerService) Stop(ctx context.Context) (err error) {
	var errs []error
	var cancel context.CancelFunc
	if d.cfg.ServerConfig.GracefulShutdownTimeout.Duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, d.cfg.ServerConfig.GracefulShutdownTimeout.Duration)
		defer cancel()
	}
	if d.minKAPIServer != nil {
		err = d.minKAPIServer.Stop(ctx)
	}
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return
}

func (p *defaultPlannerService) Plan(ctx context.Context, request plannerapi.Request) <-chan plannerapi.Response {
	return p.planner.Plan(ctx, request)
}
