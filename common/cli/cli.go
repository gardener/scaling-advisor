// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

const (
	// ExitSuccess is the exit code indicating that the CLI has exited with no error.
	ExitSuccess = iota
	// ExitErrParseOpts is the exit code indicating that the CLI has exited due to error parsing options.
	ExitErrParseOpts
	// ExitErrStart is the exit code indicating that there was an error starting the application.
	ExitErrStart
	// ExitErrShutdown is the exit code indicating that the application could not shut down cleanly.
	ExitErrShutdown = 254
)

var (
	// ErrParseArgs is a sentinel error indicating that there was an error parsing command line args.
	ErrParseArgs = errors.New("cannot parse cli args")
	// ErrMissingOpt is a sentinel error indication that there one or more required command line args are missing.
	ErrMissingOpt = errors.New("missing option")
	// ErrInvalidOpt is a sentinel error indicating that an invalid command line arg has been passed.
	ErrInvalidOpt = errors.New("invalid option value")
)

// MapServerConfigFlags adds the constants flags to the passed FlagSet.
func MapServerConfigFlags(flagSet *pflag.FlagSet, opts *commontypes.ServerConfig) {
	flagSet.StringVarP(&opts.Host, "host", "H", "", "host name to bind this service. Use 0.0.0.0 for all interfaces")
	flagSet.IntVarP(&opts.Port, "port", "P", opts.Port, "listen port for REST API")
	flagSet.BoolVarP(&opts.ProfilingEnabled, "pprof", "p", false, "enable pprof profiling")
	flagSet.DurationVar(&opts.GracefulShutdownTimeout.Duration, "shutdown-timeout", time.Second*6, "graceful shutdown timeout")

	klogFlagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlagSet)
	// Merge klog flags into pflag
	flagSet.AddGoFlagSet(klogFlagSet)
}

const (
	// DefaultQPS used when talking to kubernetes apiserver
	DefaultQPS = 50.0
	// DefaultBurst used when talking to kubernetes apiserver
	DefaultBurst = 100
)

// MapQPSBurstFlags adds the QPS and Burst values to the passed FlagSet.
func MapQPSBurstFlags(flagSet *pflag.FlagSet, opts *commontypes.QPSBurst) {
	flagSet.Float32Var(&opts.QPS, "kube-api-qps", DefaultQPS, "QPS to use while talking with kubernetes apiserver")
	flagSet.IntVar(&opts.Burst, "kube-api-burst", DefaultBurst, "Burst to use while talking with kubernetes apiserver")
}

// PrintVersion prints the version from build information for the program.
func PrintVersion(programName string) {
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" {
		fmt.Printf("%s version: %s\n", programName, info.Main.Version)
	} else {
		fmt.Printf("%s: binary build info not embedded\n", programName)
	}
}

// HandleErrorAndExit gracefully handles errors before exiting the program.
func HandleErrorAndExit(err error) {
	if errors.Is(err, pflag.ErrHelp) {
		os.Exit(ExitSuccess)
	}
	_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
	os.Exit(ExitErrParseOpts)
}

// ValidateServerConfigFlags validates server config flags.
func ValidateServerConfigFlags(opts commontypes.ServerConfig) error {
	if opts.Port <= 0 {
		return fmt.Errorf("%w: --port must be greater than 0", ErrInvalidOpt)
	}
	return nil
}

// CreateAppContext wraps the given context with a logger and signal-cancelling support and returns the same along with
// a cancellation function for the returned context.
func CreateAppContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Set up logr with klog backend using NewKlogr
	log := klog.NewKlogr()
	ctx = logr.NewContext(ctx, log)
	return ctx, stop
}
