// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"context"
	"fmt"
	"path"

	bench "github.com/gardener/scaling-advisor/bench/cmd"

	"github.com/spf13/cobra"
)

var (
	constraintsFile string
	outputDir       string
	pricingFile     string
	version         string
)

// SetupScaler defines methods needed to setup the scaler service with the
// necessary requirements needed to execute and run the benchmarking tool
type SetupScaler interface {
	// BuildScaler downloads the specified version of the scaler into
	// a temporary data directory and then builds the scaler image and
	// pushes it to docker
	BuildScaler(ctx context.Context) error
	// GenerateKwokData uses the scaling constraints file to construct
	// the relevant data required by the kwok provider of the scaler
	GenerateKwokData(ctx context.Context, constraintsFile, outputDir string) error
}

var setupCmd = &cobra.Command{
	Use:   "setup <scaler> <options>",
	Short: "Setup the scaler by fetching the required version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		scalerName := args[0]
		s, err := getScaler(scalerName)
		if err != nil {
			return
		}

		ctx := cmd.Context()
		outputDir = path.Dir(constraintsFile)
		if err = s.GenerateKwokData(ctx, constraintsFile, outputDir); err != nil {
			return fmt.Errorf("error generating kwok data for %s: %v", scalerName, err)
		}

		if err = s.BuildScaler(ctx); err != nil {
			return fmt.Errorf("error building %s source: %v", scalerName, err)
		}

		return nil
	},
}

func init() {
	bench.RootCmd.AddCommand(setupCmd)

	setupCmd.PersistentFlags().StringVarP(
		&constraintsFile,
		"constraints", "c",
		"",
		"constraints file path (required)",
	)
	_ = setupCmd.MarkFlagRequired("constraints")

	setupCmd.PersistentFlags().StringVarP(
		&pricingFile,
		"pricing-data", "p",
		"",
		"pricing data file (required for karpenter)",
	)

	setupCmd.PersistentFlags().StringVarP(
		&version,
		"scaler-version", "v",
		"main",
		"version of the scaler to fetch",
	)
}

func getScaler(scalerName string) (SetupScaler, error) {
	switch scalerName {
	case bench.ScalerKarpenter:
		if pricingFile == "" {
			return nil, fmt.Errorf("pricing data needed for karpenter: run `scadctl genprice` to get the data")
		}
		return &karpenterSetup{}, nil
	case bench.ScalerClusterAutoscaler:
		return &caSetup{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}
