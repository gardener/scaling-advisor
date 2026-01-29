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
	"time"

	"github.com/gardener/scaling-advisor/service/internal/core/util"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/ioutil"
	mkcore "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/minkapi/server/configtmpl"
	"github.com/gardener/scaling-advisor/planner"
	"github.com/gardener/scaling-advisor/planner/scheduler"
)

var _ service.ScalingAdvisorService = (*defaultScalingAdvisor)(nil)

type defaultScalingAdvisor struct {
	minKAPIServer     minkapi.Server
	schedulerLauncher plannerapi.SchedulerLauncher
	planner           plannerapi.ScalingPlanner
	cfg               service.ScalingAdvisorServiceConfig
}

// NewService initializes and returns a ScalingAdvisorService based on the provided dependencies.
func NewService(ctx context.Context,
	config service.ScalingAdvisorServiceConfig,
	pricingAccess plannerapi.InstancePricingAccess,
	weightsFn plannerapi.GetResourceWeightsFunc) (svc service.ScalingAdvisorService, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", service.ErrInitFailed, err)
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
	svc = &defaultScalingAdvisor{
		cfg:               config,
		minKAPIServer:     minKAPIServer,
		schedulerLauncher: schedulerLauncher,
		planner:           p,
	}
	return
}

func (d *defaultScalingAdvisor) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", service.ErrStartFailed, err)
		}
	}()
	if err = d.minKAPIServer.Start(ctx); err != nil {
		return
	}
	return
}

func (d *defaultScalingAdvisor) Stop(ctx context.Context) (err error) {
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

func (d *defaultScalingAdvisor) GenerateAdvice(ctx context.Context, request plannerapi.ScalingAdviceRequest) <-chan plannerapi.ScalingAdviceResult {
	genCtx, cancelFn := wrapGenerationContext(ctx, request)
	defer cancelFn()
	adviceResultCh := make(chan plannerapi.ScalingAdviceResult, 1)
	planResultCh := make(chan plannerapi.ScalingPlanResult)
	go func() {
		defer close(planResultCh)
		if len(request.Snapshot.GetUnscheduledPods()) == 0 {
			sendAdviceError(adviceResultCh, request.ScalingAdviceRequestRef, fmt.Errorf("%w: no unscheduled pods found", plannerapi.ErrNoUnscheduledPods))
			return
		}
		d.planner.Plan(genCtx, request, planResultCh)
	}()

	go func() {
		defer close(adviceResultCh)
		for planResult := range planResultCh {
			if planResult.Err != nil {
				sendAdviceError(adviceResultCh, request.ScalingAdviceRequestRef, planResult.Err)
				continue
			}
			adviceResultCh <- plannerapi.ScalingAdviceResult{
				Response: util.CreateScalingAdviceResponse(request, planResult),
			}
		}
	}()

	return adviceResultCh
}

// wrapGenerationContext wraps the given context with a timeout derived from the request's AdviceGenerationTimeout field.
// TODO instead of hard coding a 5 min default timeout, this should be computed dynamically based on the cluster snapshot and cluster constraints.
func wrapGenerationContext(ctx context.Context, request plannerapi.ScalingAdviceRequest) (context.Context, context.CancelFunc) {
	genTimeout := 5 * time.Minute
	if request.AdviceGenerationTimeout > 0 {
		genTimeout = request.AdviceGenerationTimeout
	}
	return context.WithTimeout(ctx, genTimeout)
}

func setServiceConfigDefaults(cfg *service.ScalingAdvisorServiceConfig) {
	if strings.TrimSpace(cfg.ServerConfig.BindAddress) == "" {
		cfg.ServerConfig.BindAddress = commonconstants.DefaultAdvisorServiceBindAddress
	}
	if cfg.TraceLogBaseDir == "" {
		cfg.TraceLogBaseDir = ioutil.GetTempDir()
	}
}

// sendAdviceError wraps the given error with request ref info, embeds the wrapped error within a ScalingAdviceResult and sends the same to the given results channel.
func sendAdviceError(resultsCh chan<- plannerapi.ScalingAdviceResult, requestRef plannerapi.ScalingAdviceRequestRef, err error) {
	if !errors.Is(err, plannerapi.ErrGenScalingPlan) {
		err = plannerapi.AsScalingAdviceError(requestRef.ID, requestRef.CorrelationID, err)
	}
	resultsCh <- plannerapi.ScalingAdviceResult{
		Err: err,
	}
}
