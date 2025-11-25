// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	scenarioDir     string
	outputDir       string
	version         string
	skipScalarBuild bool
)

var setupCmd = &cobra.Command{
	Use:   "setup <scaler> <options>",
	Short: "Setup the scaler by fetching the required version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("benchmark setup called")
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)

	setupCmd.PersistentFlags().StringVarP(
		&scenarioDir,
		"scenario-dir", "d",
		"",
		"scenario data directory (required)",
	)
	_ = setupCmd.MarkFlagRequired("scenario-dir")

	setupCmd.PersistentFlags().StringVarP(
		&outputDir,
		"output-dir", "o",
		"/tmp",
		"output data directory",
	)

	setupCmd.PersistentFlags().StringVarP(
		&version,
		"scaler-version", "v",
		"main",
		"version of the scaler to fetch",
	)

	setupCmd.PersistentFlags().BoolVarP(
		&skipScalarBuild,
		"skip-build", "s",
		false,
		"skip fetching and building the specified scalar",
	)
}

// SetupScaler defines methods needed to setup the scaler service with the
// necessary requirements needed to execute and run the benchmarking tool
type SetupScaler interface {
	// FetchScaler downloads the specified version of the scaler into
	// the specified data directory with a fallback if none is specified
	// TODO: ensure no redundant downloads if the required data is present
	BuildScaler(ctx context.Context, version, scaler, dataDir string) error
	// GenerateKwokData uses the cluster snapshot and scaling constraints
	// data present in scenarioDir to construct the relevant data required
	// by the kwok provider of the scaler
	GenerateKwokData(ctx context.Context, scenarioDir, outputDir string) error
}
