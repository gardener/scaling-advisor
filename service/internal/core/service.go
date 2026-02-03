// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	"github.com/gardener/scaling-advisor/common/ioutil"
	mkcore "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/minkapi/server/configtmpl"
	"github.com/gardener/scaling-advisor/planner"
	"github.com/gardener/scaling-advisor/planner/scheduler"
)

var _ plannerapi.ScalingPlannerService = (*defaultPlannerService)(nil)

type defaultPlannerService struct {
	minKAPIServer     minkapi.Server
	schedulerLauncher plannerapi.SchedulerLauncher
	planner           plannerapi.ScalingPlanner
	cfg               plannerapi.ScalingPlannerServiceConfig
}

// NewService initializes and returns a ScalingAdvisorService based on the provided dependencies.
func NewService(ctx context.Context,
	config plannerapi.ScalingPlannerServiceConfig,
	pricingAccess pricingapi.InstancePricingAccess,
	weightsFn plannerapi.GetResourceWeightsFunc) (svc plannerapi.ScalingPlannerService, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", plannerapi.ErrServiceInitFailed, err)
		}
	}()
	setServiceConfigDefaults(&config)
	minKAPIServer, err := mkcore.New(ctx, config.MinKAPIConfig)
	if err != nil {
		return
	}
	embeddedSchedulerConfigPath := path.Join(os.TempDir(), "embedded-scheduler-cfg.yaml")
	err = configtmpl.GenKubeSchedulerConfig(configtmpl.KubeSchedulerTmplParams{
		KubeConfigPath:          config.MinKAPIConfig.KubeConfigPath,
		KubeSchedulerConfigPath: embeddedSchedulerConfigPath,
		QPS:                     0,
		Burst:                   0,
	})
	if err != nil {
		return
	}
	schedulerLauncher, err := scheduler.NewLauncher(embeddedSchedulerConfigPath, config.SimulatorConfig.MaxParallelSimulations)
	if err != nil {
		return
	}
	p := planner.New(plannerapi.ScalingPlannerArgs{
		ViewAccess:        minKAPIServer,
		ResourceWeigher:   weightsFn,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		TraceLogsBaseDir:  config.TraceLogBaseDir,
	})
	svc = &defaultPlannerService{
		cfg:               config,
		minKAPIServer:     minKAPIServer,
		schedulerLauncher: schedulerLauncher,
		planner:           p,
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
		// It is possible that ctx is already a shutdown context where advisor core is embedded into a higher-level core
		// whose Stop has already created a shutdown context prior to invoking advisor core.Stop
		// In such a case, it is expected that cfg.GracefulShutdownTimeout for advisor core would not be explicitly specified.
		ctx, cancel = context.WithTimeout(ctx, d.cfg.ServerConfig.GracefulShutdownTimeout.Duration)
		defer cancel()
	}
	// TODO: Stop the scaling advisor http server.
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

func setServiceConfigDefaults(cfg *plannerapi.ScalingPlannerServiceConfig) {
	if strings.TrimSpace(cfg.ServerConfig.BindAddress) == "" {
		cfg.ServerConfig.BindAddress = commonconstants.DefaultAdvisorServiceBindAddress
	}
	if cfg.TraceLogBaseDir == "" {
		cfg.TraceLogBaseDir = ioutil.GetTempDir()
	}
}
