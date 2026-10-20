// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/gardener/scaling-advisor/service/internal/core"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	apicomponent "github.com/gardener/scaling-advisor/api/common/component"
	"github.com/gardener/scaling-advisor/common/cliutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	mkcli "github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/planner"
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
)

// App represents an application process `scaling-planner` that wraps a ScalingPlannerService, an application context
// and application cancel func.
type App struct {
	Service plannerapi.ScalingPlannerService
	Ctx     context.Context
	Cancel  context.CancelFunc
}

// Opts is a struct that encapsulates target fields for CLI options parsing.
type Opts struct {
	CloudProvider    string
	TraceDir         string
	ServerConfig     apicomponent.ServerConfig
	SimulationConfig plannerapi.SimulatorConfig
	ClientConfig     apicomponent.QPSBurst
	WatchConfig      minkapi.WatchConfig
}

// ParseProgramFlags parses the command line arguments and returns Opts.
func ParseProgramFlags(args []string) (*Opts, error) {
	flagSet, opts := setupFlagsToOpts()
	err := flagSet.Parse(args)
	if err != nil {
		return nil, err
	}
	err = opts.validate()
	if err != nil {
		return nil, err
	}
	return opts, nil
}

// LaunchApp parses CLI args, constructs and starts the ScalingPlannerService, and embeds it in an App.
func LaunchApp(ctx context.Context) (app App, exitCode int, err error) {
	defer func() {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}()

	cliOpts, err := ParseProgramFlags(os.Args[1:])
	if err != nil {
		exitCode = cliutil.ExitErrParseOpts
		return
	}

	app.Ctx, app.Cancel = cliutil.NewAppContext(ctx, plannerapi.ServiceName)
	log := logr.FromContextOrDiscard(app.Ctx)
	cliutil.PrintVersion(plannerapi.ServiceName)
	embeddedMinKAPIKubeConfigPath := path.Join(os.TempDir(), "embedded-minkapi.yaml")
	log.Info("embedded minkapi-kube cfg path", "kubeConfigPath", embeddedMinKAPIKubeConfigPath)
	cfg := plannerapi.ScalingPlannerServiceConfig{
		ServerConfig: cliOpts.ServerConfig,
		MinKAPIConfig: minkapi.Config{
			BasePrefix: minkapi.DefaultBasePrefix,
			ServerConfig: apicomponent.ServerConfig{
				BindAddress:             cliOpts.ServerConfig.BindAddress,
				KubeConfigPath:          embeddedMinKAPIKubeConfigPath,
				ProfilingEnabled:        cliOpts.ServerConfig.ProfilingEnabled,
				GracefulShutdownTimeout: cliOpts.ServerConfig.GracefulShutdownTimeout,
			},
			WatchConfig: cliOpts.WatchConfig,
		},
		ClientConfig:    cliOpts.ClientConfig,
		CloudProvider:   apicommon.CloudProvider(cliOpts.CloudProvider),
		SimulatorConfig: cliOpts.SimulationConfig,
		TraceDir:        cliOpts.TraceDir,
	}
	app.Service, err = core.NewService(app.Ctx, cfg, planner.NewFactories())
	if err != nil {
		exitCode = cliutil.ExitErrStart
		return
	}
	go func() {
		if err = app.Service.Start(app.Ctx); err != nil {
			log.Error(err, "failed to start planner")
		}
	}()
	if err != nil {
		exitCode = cliutil.ExitErrStart
		return
	}
	return
}

// ShutdownApp gracefully stops the App.
func ShutdownApp(app *App) (exitCode int) {
	log := logr.FromContextOrDiscard(app.Ctx)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := app.Service.Stop(ctx); err != nil {
		log.Error(err, fmt.Sprintf("%s shutdown failed", plannerapi.ServiceName))
		exitCode = cliutil.ExitErrShutdown
		return
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", plannerapi.ServiceName))
	exitCode = cliutil.ExitSuccess
	return
}

func (o Opts) validate() error {
	var errs []error
	errs = append(errs, cliutil.ValidateServerConfigFlags(o.ServerConfig))
	if _, err := apicommon.AsCloudProvider(o.CloudProvider); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func setupFlagsToOpts() (*pflag.FlagSet, *Opts) {
	var opts Opts
	flagSet := pflag.NewFlagSet(plannerapi.ServiceName, pflag.ContinueOnError)
	cliutil.MapServerConfigFlags(flagSet, &opts.ServerConfig)
	cliutil.MapQPSBurstFlags(flagSet, &opts.ClientConfig)
	mkcli.MapWatchConfigFlags(flagSet, &opts.WatchConfig)
	flagSet.StringVarP(&opts.CloudProvider, "cloud-provider", "c", string(apicommon.CloudProviderAWS), "cloud provider")
	flagSet.IntVarP(&opts.SimulationConfig.MaxParallelSimulations, "max-parallel-simulations", "m", plannerapi.DefaultMaxParallelSimulations, "maximum number of parallel simulations")
	flagSet.DurationVar(&opts.SimulationConfig.TrackPollInterval, "track-poll-interval", plannerapi.DefaultTrackPollInterval, "poll interval for tracking pod scheduling")
	flagSet.IntVar(&opts.SimulationConfig.MaxUnchangedTrackAttempts, "max-unchanged-track-attempts", plannerapi.DefaultMaxUnchangedTrackAttempts, "maximum unchanged simulation track attempts before stopping")
	flagSet.StringVar(&opts.TraceDir, "trace-dir", os.TempDir(), "directory for traces")
	return flagSet, &opts
}
