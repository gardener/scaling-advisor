// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"context"
	"fmt"
	"os"
	"path"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/spf13/cobra"
)

// SetupScaler defines methods needed to set up a scaler with the artefacts
// required by the benchmarking harness.
type SetupScaler interface {
	// BuildScaler downloads the specified version of the scaler into a
	// temporary data directory, builds the scaler image and loads it into
	// the local Docker daemon.
	BuildScaler(ctx context.Context, version string) error

	// GenerateKwokData uses the scaling constraints file to construct the
	// data files required by the scaler's KWOK cloud-provider.
	GenerateKwokData(ctx context.Context, constraintsFile, outputDir string) error
}

// SetupArgs contains the flag variables — passed explicitly to all callees
// so that no other function touches these globals.
type SetupArgs struct {
	Scaler          string
	ConstraintsFile string
	PricingFile     string
	Version         string
}

// NewSetupCommand is the entry point for getting the scaler for the "setup" subcommand.
func NewSetupCommand(_ context.Context) *cobra.Command {
	var setupArgs SetupArgs
	var setupCmd = &cobra.Command{
		Use:   "setup <scaler> <options>",
		Short: "Setup the scaler by fetching the required version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, cmdArgs []string) (err error) {
			cmd.SilenceUsage = true
			// Only the scaler is passed as an argument to the command, rest are all flags
			setupArgs.Scaler = cmdArgs[0]
			setupCtx := benchutil.SetupSignalHandler()
			return Run(setupCtx, setupArgs)
		},
	}

	// Initialise the setup args with the passed flag values,
	// falling back to default if nothing specified
	setupCmd.Flags().StringVarP(
		&setupArgs.ConstraintsFile,
		"constraints", "c", "",
		"constraints file path (required)",
	)
	_ = setupCmd.MarkFlagRequired("constraints")
	_ = setupCmd.MarkFlagFilename("constraints", "json")

	setupCmd.Flags().StringVarP(
		&setupArgs.PricingFile,
		"pricing-data", "p", "",
		"pricing data file (required)",
	)
	_ = setupCmd.MarkFlagRequired("pricing-data")
	_ = setupCmd.MarkFlagFilename("pricing-data", "json")

	setupCmd.Flags().StringVarP(
		&setupArgs.Version,
		"scaler-version", "v", "main",
		"version of the scaler to fetch (can specify tags or commitID)",
	)

	return setupCmd
}

// Run fetches and builds the scaler and prepares all artefacts that the "exec" subcommand later consumes.
func Run(ctx context.Context, args SetupArgs) (err error) {
	scaler, err := getScaler(args.Scaler, args.PricingFile)
	if err != nil {
		return err
	}

	err = benchutil.CheckIfDockerRunning()
	if err != nil {
		return fmt.Errorf("docker is not running: %v", err)
	}

	// Derive the output directory from the constraints file location so
	// that all generated artefacts live next to the input data.
	outputDir := path.Join(path.Dir(args.ConstraintsFile), "gen")
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("could not create the output directory (%s): %v", outputDir, err)
	}
	// Link the pricing file in the generated directory, to allow for usage
	// during harness execution. Remove any existing link so re-runs succeed.
	pricingDst := path.Join(outputDir, benchutil.FileNamePricingData)
	_ = os.Remove(pricingDst)
	if err := os.Link(args.PricingFile, pricingDst); err != nil {
		return fmt.Errorf("could not link pricing file to output directory: %v", err)
	}

	if err := scaler.GenerateKwokData(ctx, args.ConstraintsFile, outputDir); err != nil {
		return fmt.Errorf("error generating kwok data for %s: %v", args.Scaler, err)
	}
	if err := scaler.BuildScaler(ctx, args.Version); err != nil {
		return fmt.Errorf("error building %s source: %v", args.Scaler, err)
	}

	if err := benchutil.PullDockerImage("prom/prometheus:latest"); err != nil {
		return fmt.Errorf("error pulling prometheus image: %v", err)
	}

	return nil
}

func getScaler(scalerName, pricingFile string) (SetupScaler, error) {
	switch scalerName {
	case benchutil.ScalerKarpenter:
		return &karpenterSetup{pricingFile: pricingFile}, nil
	case benchutil.ScalerClusterAutoscaler:
		return &caSetup{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}
