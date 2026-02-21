package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/gardener/scaling-advisor/service/internal/core"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/api/service"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	mkcli "github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/planner"
	"github.com/gardener/scaling-advisor/pricing"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
)

// App represents an application process `scaling-planner` that wraps a ScalingPlannerService, an application context
// and application cancel func. `main` entry-point functions that embed ScalingPlannerService are expected to launch a new App
// instance via cli.LaunchApp and shutdown the instance via cli.ShutdownApp.
type App struct {
	// Service is the scaling planner service.
	Service plannerapi.ScalingPlannerService
	// Ctx is the application context.
	Ctx context.Context
	// Cancel is the context cancellation function.
	Cancel context.CancelFunc
}

// Opts is a struct that encapsulates target fields for CLI options parsing.
type Opts struct {
	InstancePricingPath string
	// CloudProvider is the cloud provider for which the scaling advisor planner is initialized.
	CloudProvider    string
	TraceDir         string
	ServerConfig     commontypes.ServerConfig
	ClientConfig     commontypes.QPSBurst
	WatchConfig      minkapi.WatchConfig
	SimulationConfig planner.SimulatorConfig
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

// LaunchApp is a helper function used to parse cli args, construct, and start the ScalingAdvisorService,
// embed the planner inside an App representing the binary process along with an application context and application cancel func.
//
// On success, returns an initialized App which holds the ScalingAdvisorService, the App Context (set up for SIGINT and SIGTERM cancellation and holds a logger),
// and the Cancel func which callers are expected to defer in their main routines.
//
// On error, it will log the error to standard error and return the exitCode that callers are expected to exit the process with.
func LaunchApp(ctx context.Context) (app service.App, exitCode int, err error) {
	defer func() {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
	}()

	cliOpts, err := ParseProgramFlags(os.Args[1:])
	if err != nil {
		exitCode = commoncli.ExitErrParseOpts
		return
	}

	app.Ctx, app.Cancel = commoncli.CreateAppContext(ctx, service.ProgramName)
	log := logr.FromContextOrDiscard(app.Ctx)
	commoncli.PrintVersion(service.ProgramName)
	embeddedMinKAPIKubeConfigPath := path.Join(os.TempDir(), "embedded-minkapi.yaml")
	log.Info("embedded minkapi-kube cfg path", "kubeConfigPath", embeddedMinKAPIKubeConfigPath)
	cloudProvider, err := commontypes.AsCloudProvider(cliOpts.CloudProvider)
	if err != nil {
		exitCode = commoncli.ExitErrParseOpts
		return
	}
	cfg := service.ScalingAdvisorServiceConfig{
		ServerConfig: cliOpts.ServerConfig,
		MinKAPIConfig: minkapi.Config{
			BasePrefix: minkapi.DefaultBasePrefix,
			ServerConfig: commontypes.ServerConfig{
				BindAddress:             cliOpts.ServerConfig.BindAddress,
				KubeConfigPath:          embeddedMinKAPIKubeConfigPath,
				ProfilingEnabled:        cliOpts.ServerConfig.ProfilingEnabled,
				GracefulShutdownTimeout: cliOpts.ServerConfig.GracefulShutdownTimeout,
			},
			WatchConfig: cliOpts.WatchConfig,
		},
		ClientConfig:    cliOpts.ClientConfig,
		CloudProvider:   cloudProvider,
		SimulatorConfig: cliOpts.SimulationConfig,
		TraceDir:        cliOpts.TraceDir,
	}
	pricingAccess, err := pricing.GetInstancePricingAccess(cloudProvider, cliOpts.InstancePricingPath)
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	app.Service, err = core.NewService(app.Ctx, cfg, pricingAccess, planner.NewFactories())
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	// Begin the planner in a goroutine
	go func() {
		if err = app.Service.Start(app.Ctx); err != nil {
			log.Error(err, "failed to start planner")
		}
	}()
	if err != nil {
		exitCode = commoncli.ExitErrStart
		return
	}
	return
}

// ShutdownApp gracefully stops the App process wrapping the ScalingAdvisorService and returns an exit code.
func ShutdownApp(app *service.App) (exitCode int) {
	log := logr.FromContextOrDiscard(app.Ctx)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := app.Service.Stop(ctx); err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", minkapi.ProgramName))
		exitCode = commoncli.ExitErrShutdown
		return
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", minkapi.ProgramName))
	exitCode = commoncli.ExitSuccess
	return
}

func (o Opts) validate() error {
	var errs []error
	errs = append(errs, commoncli.ValidateServerConfigFlags(o.ServerConfig))
	if len(strings.TrimSpace(o.ServerConfig.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", commonerrors.ErrMissingOpt))
	}
	if len(strings.TrimSpace(o.ServerConfig.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", commonerrors.ErrMissingOpt))
	}
	if len(o.InstancePricingPath) == 0 {
		errs = append(errs, fmt.Errorf("%w: --instance-pricing/-i", commonerrors.ErrMissingOpt))
	}
	_, err := commontypes.AsCloudProvider(o.CloudProvider)
	if err != nil {
		errs = append(errs, err)
	}
	if len(o.InstancePricingPath) > 0 {
		fInfo, err := os.Stat(o.InstancePricingPath)
		if err != nil {
			err = fmt.Errorf("%w: --instance-pricing/-k should exist and be readable: %w", commonerrors.ErrInvalidOptVal, err)
			errs = append(errs, err)
		}
		if fInfo.IsDir() {
			err = fmt.Errorf("%w: --instance-pricing/-k should be a file", commonerrors.ErrInvalidOptVal)
			errs = append(errs, err)
		}
		if fInfo.Size() == 0 {
			err = fmt.Errorf("%w: --instance-pricing/-k should not be empty", commonerrors.ErrInvalidOptVal)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func setupFlagsToOpts() (*pflag.FlagSet, *Opts) {
	var opts Opts
	flagSet := pflag.NewFlagSet(service.ProgramName, pflag.ContinueOnError)
	commoncli.MapServerConfigFlags(flagSet, &opts.ServerConfig)
	commoncli.MapQPSBurstFlags(flagSet, &opts.ClientConfig)
	mkcli.MapWatchConfigFlags(flagSet, &opts.WatchConfig)
	flagSet.StringVar(&opts.InstancePricingPath, "instance-info", "", "path to instance info file (contains prices)")
	flagSet.StringVarP(&opts.CloudProvider, "cloud-provider", "c", string(commontypes.CloudProviderAWS), "cloud provider")
	flagSet.IntVarP(&opts.SimulationConfig.MaxParallelSimulations, "max-parallel-simulations", "m", plannerapi.DefaultMaxParallelSimulations, "maximum number of parallel simulations")
	flagSet.DurationVar(&opts.SimulationConfig.TrackPollInterval, "track-poll-interval", plannerapi.DefaultTrackPollInterval, "poll interval for tracking pod scheduling in the view of the simulator")
	flagSet.StringVar(&opts.TraceDir, "trace-dir", os.TempDir(), "directory for traces ")
	flagSet.StringVarP(&opts.InstancePricingPath, "pricing", "p", "", "path to instance pricing file")
	return flagSet, &opts
}
