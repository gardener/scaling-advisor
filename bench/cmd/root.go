// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"fmt"
	"os"

	"github.com/gardener/scaling-advisor/bench/cmd/exec"
	"github.com/gardener/scaling-advisor/bench/cmd/setup"

	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
// For the bench module, the command is 'scalebench'
var RootCmd = &cobra.Command{
	Use:   "scalebench <mode> <scaler> <options>",
	Short: "Benchmark performance of the specified scaler for the provided scenario",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx := RootCmd.Context()
	RootCmd.AddCommand(setup.NewSetupCommand(ctx))
	RootCmd.AddCommand(exec.NewExecCommand(ctx))

	err := RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
