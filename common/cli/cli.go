// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
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
	flagSet.StringVarP(&opts.KubeConfigPath, clientcmd.RecommendedConfigPathFlag, "k", opts.KubeConfigPath, "path to master kubeconfig - fallback to KUBECONFIG env-var")
	flagSet.StringVarP(&opts.Host, "host", "H", "", "host name to bind this service. Use 0.0.0.0 for all interfaces")
	flagSet.IntVarP(&opts.Port, "port", "P", 0, "listen port for REST API")
	flagSet.BoolVarP(&opts.ProfilingEnabled, "pprof", "p", false, "enable pprof profiling")

	klogFlagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlagSet)
	// Merge klog flags into pflag
	flagSet.AddGoFlagSet(klogFlagSet)
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
