// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// genscenarioCmd represents the genscenario command
var genscenarioCmd = &cobra.Command{
	Use:   "genscenario <cluster-manager>",
	Short: "Generate scaling scenarios for the given cluster-manager",
	Long: `genscenario generates scaling scenarios for the given cluster-manager.
Generate scaling scenario(s) for the gardener cluster identified by the given gardener landscape, gardener project
and gardener shoot name and write the scenario(s) to the scenario-dir.
	 genscenario gardener -l <landscape> -p <project> -t <shoot-name> -d <scenario-dir>
`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("genscenario called")
	},
}

func init() {
	rootCmd.AddCommand(genscenarioCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// genscenarioCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// genscenarioCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
