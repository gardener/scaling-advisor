// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
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

func setupFlagsToOpts() (*pflag.FlagSet, *Opts) {
	var opts Opts
	flagSet := pflag.NewFlagSet(minkapi.ProgramName, pflag.ContinueOnError)

	opts.KubeConfigPath = os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if opts.KubeConfigPath == "" {
		opts.KubeConfigPath = minkapi.DefaultKubeConfigPath
	}
	if opts.Port == 0 {
		opts.Port = commonconstants.DefaultMinKAPIPort
	}
	// TODO: Change opts.KubeConfigPath to opts.KubeConfigGenDir later
	flagSet.StringVarP(&opts.KubeConfigPath, clientcmd.RecommendedConfigPathFlag, "k", opts.KubeConfigPath, "path to master kubeconfig - fallback to KUBECONFIG env-var")
	commoncli.MapServerConfigFlags(flagSet, &opts.ServerConfig)
	MapWatchConfigFlags(flagSet, &opts.WatchConfig)
	flagSet.StringVarP(&opts.BasePrefix, "base-prefix", "b", minkapi.DefaultBasePrefix, "base path prefix for the base view of the minkapi service")
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
