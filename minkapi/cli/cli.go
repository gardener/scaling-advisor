// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/minkapi"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
	"strings"
)

// MainOpts is a struct that encapsulates target fields for CLI options parsing.
type MainOpts struct {
	minkapi.Config
}

// ParseProgramFlags parses the command line arguments and returns Opts.
func ParseProgramFlags(args []string) (*Opts, error) {
	flagSet, mainOpts := setupFlagsToOpts()
	err := flagSet.Parse(args)
	if err != nil {
		return nil, err
	}
	err = validateMainOpts(mainOpts)
	if err != nil {
		return nil, err
	}
	return mainOpts, nil
}

func setupFlagsToOpts() (*pflag.FlagSet, *MainOpts) {
	var mainOpts MainOpts
	flagSet := pflag.NewFlagSet(minkapi.ProgramName, pflag.ContinueOnError)

	mainOpts.KubeConfigPath = os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if mainOpts.KubeConfigPath == "" {
		mainOpts.KubeConfigPath = minkapi.DefaultKubeConfigPath
	}
	app.Ctx, app.Cancel = commoncli.NewAppContext(ctx, minkapi.ProgramName)
	log := logr.FromContextOrDiscard(app.Ctx)
	app.Server, err = server.New(ctx, cliOpts.Config)
	if err != nil {
		log.Error(err, "failed to initialize minkapi server")
		exitCode = commoncli.ExitErrStart
		return
	}
	commoncli.MapServerConfigFlags(flagSet, &mainOpts.ServerConfig)
	flagSet.IntVarP(&mainOpts.WatchConfig.QueueSize, "watch-queue-size", "s", minkapi.DefaultWatchQueueSize, "max number of events to queue per watcher")
	flagSet.DurationVarP(&mainOpts.WatchConfig.Timeout, "watch-timeout", "t", minkapi.DefaultWatchTimeout, "watch timeout after which connection is closed and watch removed")
	flagSet.StringVarP(&mainOpts.BasePrefix, "base-prefix", "b", minkapi.DefaultBasePrefix, "base path prefix for the base view of the minkapi service")

	klogFlagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlagSet)

	// Merge klog flags into pflag
	flagSet.AddGoFlagSet(klogFlagSet)

	return flagSet, &mainOpts
}

// ShutdownApp gracefully shuts-down the given minkapi application and returns an exit code that can be used by the cli hosting the app.
func ShutdownApp(app *minkapi.App) (exitCode int) {
	shutDownCtx, cancel := context.WithTimeout(context.Background(), commonconstants.DefaultGracefulShutdownTimeout)
	defer cancel()
	log := logr.FromContextOrDiscard(app.Ctx)

	// Perform shutdown
	if err := app.Server.Stop(shutDownCtx); err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", minkapi.ProgramName))
		exitCode = commoncli.ExitErrShutdown
		return
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", minkapi.ProgramName))
	exitCode = commoncli.ExitSuccess
	return
}

func setupFlagsToOpts() (*pflag.FlagSet, *Opts) {
	var opts Opts
	flagSet := pflag.NewFlagSet(minkapi.ProgramName, pflag.ContinueOnError)

	if opts.KubeConfigPath == "" {
		opts.KubeConfigPath = minkapi.DefaultKubeConfigPath
	}
	if len(opts.BindAddress) == 0 {
		opts.BindAddress = net.JoinHostPort("", strconv.Itoa(commonconstants.DefaultMinKAPIPort))
	}
	// TODO: Change opts.KubeConfigPath to opts.KubeConfigGenDir later
	flagSet.StringVarP(&opts.KubeConfigPath, clientcmd.RecommendedConfigPathFlag, "k", opts.KubeConfigPath, "path to master kubeconfig - fallback to KUBECONFIG env-var")
	commoncli.MapServerConfigFlags(flagSet, &opts.ServerConfig)
	MapWatchConfigFlags(flagSet, &opts.WatchConfig)
	flagSet.StringVarP(&opts.BasePrefix, "base-prefix", "b", minkapi.DefaultBasePrefix, "base path prefix for the base view of the minkapi core")
	return flagSet, &opts
}

// MapWatchConfigFlags  adds the watch configuration flags to the passed FlagSet.
func MapWatchConfigFlags(flagSet *pflag.FlagSet, opts *minkapi.WatchConfig) {
	flagSet.IntVarP(&opts.QueueSize, "watch-queue-size", "s", minkapi.DefaultWatchQueueSize, "max number of events to queue per watcher")
	flagSet.DurationVarP(&opts.Timeout, "watch-timeout", "t", minkapi.DefaultWatchTimeout, "watch timeout after which connection is closed and watch removed")
}

func validateMainOpts(opts *Opts) error {
	var errs []error
	errs = append(errs, commoncli.ValidateServerConfigFlags(opts.ServerConfig))
	if len(strings.TrimSpace(opts.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", minkapi.ErrMissingOpt))
	}
	return errors.Join(errs...)
}
