// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/gardener/scaling-advisor/service/internal/service/weights"
	"os"
	"path"

	"github.com/gardener/scaling-advisor/service/cli"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/gardener/scaling-advisor/service/internal/service/generator"
	"github.com/gardener/scaling-advisor/service/internal/service/simulation"
	"github.com/gardener/scaling-advisor/service/pricing"
	"github.com/gardener/scaling-advisor/service/scorer"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	mkcore "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/minkapi/server/configtmpl"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
)

var _ svcapi.ScalingAdvisorService = (*defaultScalingAdvisor)(nil)

type defaultScalingAdvisor struct {
	cfg               svcapi.ScalingAdvisorServiceConfig
	minKAPIServer     mkapi.Server
	schedulerLauncher svcapi.SchedulerLauncher
	generator         *generator.Generator
}

func New(ctx context.Context,
	config svcapi.ScalingAdvisorServiceConfig,
	pricingAccess svcapi.InstancePricingAccess,
	weightsFn svcapi.GetWeightsFunc,
	nodeScorer svcapi.NodeScorer,
	selector svcapi.NodeScoreSelector) (svc svcapi.ScalingAdvisorService, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrInitFailed, err)
		}
	}()
	setServiceConfigDefaults(&config)
	minKAPIServer, err := mkcore.NewDefaultInMemory(ctx, config.MinKAPIConfig)
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
	schedulerLauncher, err := scheduler.NewLauncher(embeddedSchedulerConfigPath, config.MaxParallelSimulations)
	if err != nil {
		return
	}
	g := generator.New(&generator.Args{
		ViewAccess:        minKAPIServer,
		PricingAccess:     pricingAccess,
		WeightsFn:         weightsFn,
		NodeScorer:        nodeScorer,
		Selector:          selector,
		SimulationCreator: svcapi.SimulationCreatorFunc(simulation.New),
		SimulationGrouper: svcapi.SimulationGrouperFunc(simulation.CreateSimulationGroups),
		SchedulerLauncher: schedulerLauncher,
	})
	svc = &defaultScalingAdvisor{
		cfg:               config,
		minKAPIServer:     minKAPIServer,
		schedulerLauncher: schedulerLauncher,
		generator:         g,
	}
	return
}

func (d *defaultScalingAdvisor) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrStartFailed, err)
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
	if d.cfg.GracefulShutdownTimeout.Duration > 0 {
		// It is possible that ctx is already a shutdown context where advisor service is embedded into a higher-level service
		// whose Stop has already created a shutdown context prior to invoking advisor service.Stop
		// In such a case, it is expected that cfg.GracefulShutdownTimeout for advisor service would not be explicitly specified.
		ctx, cancel = context.WithTimeout(ctx, d.cfg.GracefulShutdownTimeout.Duration)
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

func (d *defaultScalingAdvisor) GenerateAdvice(ctx context.Context, request svcapi.ScalingAdviceRequest) <-chan svcapi.ScalingAdviceResult {
	resultsCh := make(chan svcapi.ScalingAdviceResult)
	go func() {
		if len(request.Snapshot.GetUnscheduledPods()) == 0 {
			generator.SendError(resultsCh, request.ScalingAdviceRequestRef, fmt.Errorf("%w: no unscheduled pods found", svcapi.ErrNoUnscheduledPods))
			return
		}
		genCtx := logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", request.ID, "correlationID", request.CorrelationID))
		runArgs := &generator.RunArgs{
			Request:   request,
			ResultsCh: resultsCh,
		}
		d.generator.Run(genCtx, runArgs)
	}()
	return resultsCh
}

// LaunchApp is a helper function used to parse cli args, construct, and start the ScalingAdvisorService,
// embed the service inside an App representing the binary process along with an application context and application cancel func.
//
// On success, returns an initialized App which holds the ScalingAdvisorService, the App Context (which has been setup for SIGINT and SIGTERM cancellation and holds a logger),
// and the Cancel func which callers are expected to defer in their main routines.
//
// On error, it will log the error to standard error and return the exitCode that callers are expected to exit the process with.
func LaunchApp(ctx context.Context) (app svcapi.App, exitCode int) {
	var err error
	defer func() {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
	}()

	app.Ctx, app.Cancel = commoncli.CreateAppContext(ctx)
	log := logr.FromContextOrDiscard(app.Ctx).WithValues("program", svcapi.ProgramName)
	commoncli.PrintVersion(svcapi.ProgramName)
	cliOpts, err := cli.ParseProgramFlags(os.Args[1:])
	if err != nil {
		exitCode = commoncli.ExitErrParseOpts
		return
	}
	embeddedMinKAPIKubeConfigPath := path.Join(os.TempDir(), "embedded-minkapi.yaml")
	log.Info("embedded minkapi-kube cfg path", "kubeConfigPath", embeddedMinKAPIKubeConfigPath)
	cloudProvider, err := commontypes.AsCloudProvider(cliOpts.CloudProvider)
	if err != nil {
		exitCode = commoncli.ExitErrParseOpts
		return
	}
	cfg := svcapi.ScalingAdvisorServiceConfig{
		ServerConfig: commontypes.ServerConfig{
			HostPort: commontypes.HostPort{
				Host: cliOpts.Host,
				Port: cliOpts.Port,
			},
			ProfilingEnabled:        cliOpts.ProfilingEnabled,
			GracefulShutdownTimeout: cliOpts.GracefulShutdownTimeout,
		},
		MinKAPIConfig: mkapi.Config{
			BasePrefix: mkapi.DefaultBasePrefix,
			ServerConfig: commontypes.ServerConfig{
				HostPort: commontypes.HostPort{
					Host: "localhost",
					Port: commonconstants.DefaultMinKAPIPort,
				},
				KubeConfigPath:   embeddedMinKAPIKubeConfigPath,
				ProfilingEnabled: cliOpts.ProfilingEnabled,
			},
			WatchConfig: cliOpts.WatchConfig,
		},
		QPSBurst:               cliOpts.QPSBurst,
		CloudProvider:          cloudProvider,
		MaxParallelSimulations: cliOpts.MaxParallelSimulations,
	}
	pricingAccess, err := pricing.GetInstancePricingAccess(cloudProvider, cliOpts.InstancePricingPath)
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	weightsFn := weights.GetDefaultWeightsFn()
	nodeScorer, err := scorer.GetNodeScorer(commontypes.LeastCostNodeScoringStrategy, pricingAccess, weightsFn)
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	// TODO: ask meghna whether this can be made an interface and if weightsFn can be passed at construction time.
	nodeSelector, err := scorer.GetNodeScoreSelector(commontypes.LeastCostNodeScoringStrategy)
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	app.Service, err = New(app.Ctx, cfg, pricingAccess, weightsFn, nodeScorer, nodeSelector)
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	// Begin the service in a goroutine
	go func() {
		if err = app.Service.Start(app.Ctx); err != nil {
			log.Error(err, "failed to start service")
		}
	}()
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	return
}

func setServiceConfigDefaults(cfg *svcapi.ScalingAdvisorServiceConfig) {
	if cfg.Port == 0 {
		cfg.Port = commonconstants.DefaultAdvisorServicePort
	}
}

func ShutdownApp(app *svcapi.App) (exitCode int) {
	log := logr.FromContextOrDiscard(app.Ctx)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := app.Service.Stop(ctx); err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", mkapi.ProgramName))
		exitCode = commoncli.ExitErrShutdown
		return
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", mkapi.ProgramName))
	exitCode = commoncli.ExitSuccess
	return
}
