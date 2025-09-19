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
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	mkcli "github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/spf13/pflag"
)

// Opts is a struct that encapsulates target fields for CLI options parsing.
type Opts struct {
	commontypes.ServerConfig
	commontypes.QPSBurst
	WatchConfig         minkapi.WatchConfig
	InstancePricingPath string
	// CloudProvider is the cloud provider for which the scaling advisor service is initialized.
	CloudProvider          string
	MaxParallelSimulations int
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

func validateMainOpts(opts *Opts) error {
	var errs []error
	errs = append(errs, commoncli.ValidateServerConfigFlags(opts.ServerConfig))
	if len(strings.TrimSpace(opts.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", commonerrors.ErrMissingOpt))
	}
	if len(strings.TrimSpace(opts.KubeConfigPath)) == 0 {
		errs = append(errs, fmt.Errorf("%w: --kubeconfig/-k", commonerrors.ErrMissingOpt))
	}
	if len(opts.InstancePricingPath) == 0 {
		errs = append(errs, fmt.Errorf("%w: --instance-pricing/-i", commonerrors.ErrMissingOpt))
	}
	_, err := commontypes.AsCloudProvider(opts.CloudProvider)
	if err != nil {
		errs = append(errs, err)
	}
	fi, err := os.Stat(opts.InstancePricingPath)
	if err != nil {
		err = fmt.Errorf("%w: --instance-pricing/-k should exist and be readable: %w", commonerrors.ErrInvalidOptVal, err)
		errs = append(errs, err)
	}
	if fi.IsDir() {
		err = fmt.Errorf("%w: --instance-pricing/-k should be a file", commonerrors.ErrInvalidOptVal)
		errs = append(errs, err)
	}
	if fi.Size() == 0 {
		err = fmt.Errorf("%w: --instance-pricing/-k should not be empty", commonerrors.ErrInvalidOptVal)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func setupFlagsToOpts() (*pflag.FlagSet, *Opts) {
	var opts Opts
	flagSet := pflag.NewFlagSet(svcapi.ProgramName, pflag.ContinueOnError)
	if opts.Port == 0 {
		opts.Port = commonconstants.DefaultAdvisorServicePort
	}
	commoncli.MapServerConfigFlags(flagSet, &opts.ServerConfig)
	commoncli.MapQPSBurstFlags(flagSet, &opts.QPSBurst)
	mkcli.MapWatchConfigFlags(flagSet, &opts.WatchConfig)
	flagSet.StringVar(&opts.InstancePricingPath, "--instance-info", "-i", "path to instance info file (contains prices)")
	flagSet.StringVarP(&opts.CloudProvider, "--cloud-provider", "-c", string(commontypes.AWSCloudProvider), "cloud provider")
	flagSet.IntVarP(&opts.MaxParallelSimulations, "--max-parallel-simulations", "-m", 1, "maximum number of parallel simulations")
	return flagSet, &opts
}
