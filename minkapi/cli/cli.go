// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/gardener/scaling-advisor/minkapi/server"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	commoncli "github.com/gardener/scaling-advisor/common/cliutil"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

// Opts is a struct that encapsulates target fields for CLI options parsing.
type Opts struct {
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

// LaunchApp is a helper function used to parse cli args, construct, and start the MinKAPI server,
// embed this inside an App representing the binary process along with an application context and application cancel func.
//
// On success, returns an initialized App which holds the minkapi Server, the App Context (which has been setup for SIGINT and SIGTERM cancellation and holds a logger),
// and the Cancel func which callers are expected to defer in their main routines.
//
// On error, it will log the error to standard error and return the exitCode that callers are expected to exit the process with.
func LaunchApp(ctx context.Context) (app minkapi.App, exitCode int, err error) {
	commoncli.PrintVersion(minkapi.ProgramName)
	var cliOpts *Opts
	cliOpts, err = ParseProgramFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitCode = commoncli.ExitErrParseOpts
		return
	}
	app.Ctx, app.Cancel = commoncli.NewAppContext(ctx, minkapi.ProgramName)
	log := logr.FromContextOrDiscard(app.Ctx)
	app.Server, err = server.New(ctx, cliOpts.Config)
	if err != nil {
		log.Error(err, "failed to initialize minkapi server")
		exitCode = commoncli.ExitErrStart
		return
	}
	// Begin the core in a goroutine
	go func() {
		err = app.Server.Start(app.Ctx)
		if err != nil {
			if errors.Is(err, minkapi.ErrStartFailed) {
				log.Error(err, "failed to start core")
			} else {
				log.Error(err, fmt.Sprintf("%s start failed", minkapi.ProgramName))
			}
		}
	}()
	return
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
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", commonerrors.ErrMissingOpt))
	}
	return errors.Join(errs...)
}
