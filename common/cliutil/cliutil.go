// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cliutil

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
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
	flagSet.StringVar(&opts.BindAddress, "bind-address", commonconstants.DefaultMinKAPIBindAddress, "bind address of the form <host>:<port>")
	flagSet.BoolVar(&opts.ProfilingEnabled, "profile", false, "enable pprof profiling")
	flagSet.DurationVar(&opts.GracefulShutdownTimeout.Duration, "shutdown-timeout", commonconstants.DefaultGracefulShutdownTimeout, "graceful shutdown timeout")

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
	_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(ExitErrParseOpts)
}

// ValidateServerConfigFlags validates server config flags.
func ValidateServerConfigFlags(opts commontypes.ServerConfig) error {
	if strings.TrimSpace(opts.BindAddress) == "" {
		return fmt.Errorf("%w: --bind-address must be specified", ErrInvalidOpt)
	}
	return nil
}

// NewAppContext wraps the given context with a logger and signal-cancelling support and returns the same along with
// a cancellation function for the returned context.
// NOTE: Should be invoked only AFTER parsing program flags, so that the logger instance is initialized with logger flags.
func NewAppContext(ctx context.Context, programName string) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Set up logr with klog backend using NewKlogr
	log := klog.NewKlogr().WithValues("program", programName)
	ctx = logr.NewContext(ctx, log)
	return ctx, stop
}
