// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/gardener/scaling-advisor/api/common/constants"
	configv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"
	configv1alpha1validation "github.com/gardener/scaling-advisor/api/config/v1alpha1/validation"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
)

var ErrLoadOperatorConfig = fmt.Errorf("cannot load %q operator config", constants.OperatorName)

type LaunchOptions struct {
	ConfigFile string
	Version    bool
}

// ParseLaunchOptions parses the CLI arguments for the scaling-advisor operator.
func ParseLaunchOptions(cliArgs []string) (*LaunchOptions, error) {
	launchOpts := &LaunchOptions{}
	flagSet := pflag.NewFlagSet(constants.OperatorName, pflag.ContinueOnError)
	launchOpts.mapFlags(flagSet)
	err := flagSet.Parse(cliArgs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", commoncli.ErrParseArgs, err)
	}
	return launchOpts, nil
}

func (o *LaunchOptions) ValidateAndLoadOperatorConfig() (*configv1alpha1.ScalingAdvisorConfiguration, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	operatorConfig, err := o.loadOperatorConfig()
	if err != nil {
		return nil, err
	}
	if errs := configv1alpha1validation.ValidateScalingAdvisorConfiguration(operatorConfig); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return operatorConfig, nil
}

// loadOperatorConfig loads the operator configuration from the ConfigFile specified in the LaunchOptions.
func (o *LaunchOptions) loadOperatorConfig() (*configv1alpha1.ScalingAdvisorConfiguration, error) {
	configScheme := runtime.NewScheme()
	if err := configv1alpha1.AddToScheme(configScheme); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadOperatorConfig, err)
	}
	operatorConfig := &configv1alpha1.ScalingAdvisorConfiguration{}
	if err := objutil.LoadYAMLIntoRuntimeObject(o.ConfigFile, configScheme, operatorConfig); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadOperatorConfig, err)
	}
	return operatorConfig, nil
}

func (o *LaunchOptions) validate() error {
	if len(o.ConfigFile) == 0 && !o.Version {
		return fmt.Errorf("%w: one of version or config should be specified", commoncli.ErrMissingOpt)
	}
	if len(o.ConfigFile) > 0 && o.Version {
		return fmt.Errorf("%w: both config and version cannot be specified", commoncli.ErrInvalidOpt)
	}
	return nil
}

func (o *LaunchOptions) mapFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ConfigFile, "config", o.ConfigFile, "path to the config file")
	fs.BoolVarP(&o.Version, "version", "V", o.Version, "print version and exit")
}
