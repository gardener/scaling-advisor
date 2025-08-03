// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gardener/scaling-advisor/minkapi/api"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// MainOpts is a struct that encapsulates target fields for CLI options parsing.
type MainOpts struct {
	api.MinKAPIConfig
}

// ParseProgramFlags parses the command line arguments and returns MainOpts.
func ParseProgramFlags(args []string) (*MainOpts, error) {
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
	flagSet := pflag.NewFlagSet(api.ProgramName, pflag.ContinueOnError)

	mainOpts.KubeConfigPath = os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if mainOpts.KubeConfigPath == "" {
		mainOpts.KubeConfigPath = api.DefaultKubeConfigPath
	}
	commoncli.MapServerConfigFlags(flagSet, &mainOpts.ServerConfig)
	flagSet.IntVarP(&mainOpts.WatchQueueSize, "watch-queue-size", "s", api.DefaultWatchQueueSize, "max number of events to queue per watcher")
	flagSet.DurationVarP(&mainOpts.WatchTimeout, "watch-timeout", "t", api.DefaultWatchTimeout, "watch timeout after which connection is closed and watch removed")

	klogFlagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlagSet)

	// Merge klog flags into pflag
	flagSet.AddGoFlagSet(klogFlagSet)

	return flagSet, &mainOpts
}

func validateMainOpts(opts *MainOpts) error {
	var errs []error
	errs = append(errs, commoncli.ValidateServerConfigFlags(opts.ServerConfig))
	if len(strings.TrimSpace(opts.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", api.ErrMissingOpt))
	}
	return errors.Join(errs...)
}
