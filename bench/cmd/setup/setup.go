// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"

	bench "github.com/gardener/scaling-advisor/bench/cmd"

	"github.com/spf13/cobra"
)

// Flag variables — bound by cobra, read once in setupCmd.RunE, then passed
// explicitly to all callees so that no other function touches these globals.
var (
	constraintsFile   string
	pricingFile       string
	version           string
	prometheusVersion string
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

// setupCmd is the entry point for the "setup" subcommand. It fetches, builds,
// and prepares all artefacts that the "exec" subcommand later consumes.
var setupCmd = &cobra.Command{
	Use:   "setup <scaler> <options>",
	Short: "Setup the scaler by fetching the required version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scalerName := args[0]
		scaler, err := getScaler(scalerName)
		if err != nil {
			return err
		}

		// Derive the output directory from the constraints file location so
		// that all generated artefacts live next to the input data.
		outputDir := path.Dir(constraintsFile)

		ctx := cmd.Context()
		if err := scaler.GenerateKwokData(ctx, constraintsFile, outputDir); err != nil {
			return fmt.Errorf("error generating kwok data for %s: %v", scalerName, err)
		}
		if err := scaler.BuildScaler(ctx, version); err != nil {
			return fmt.Errorf("error building %s source: %v", scalerName, err)
		}
		if err := pullPrometheusImage(prometheusVersion); err != nil {
			return fmt.Errorf("error pulling prometheus image: %v", err)
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

	setupCmd.PersistentFlags().StringVar(
		&prometheusVersion,
		"prometheus-version",
		"latest",
		"prometheus image tag to pull",
	)
}

func getScaler(scalerName string) (SetupScaler, error) {
	switch scalerName {
	case bench.ScalerKarpenter:
		if pricingFile == "" {
			return nil, fmt.Errorf("pricing data needed for karpenter: run `scadctl genprice` to get the data")
		}
		return &karpenterSetup{pricingFile: pricingFile}, nil
	case bench.ScalerClusterAutoscaler:
		return &caSetup{}, nil
	default:
		return nil, fmt.Errorf("unknown scaler %q", scalerName)
	}
}

func pullPrometheusImage(version string) error {
	image := "prom/prometheus:" + version
	check := exec.Command("docker", "image", "inspect", image)
	if check.Run() == nil {
		return nil
	}
	fmt.Printf("Pulling %s...\n", image)
	pull := exec.Command("docker", "pull", image)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		return fmt.Errorf("docker pull %s: %w", image, err)
	}
	return nil
}
