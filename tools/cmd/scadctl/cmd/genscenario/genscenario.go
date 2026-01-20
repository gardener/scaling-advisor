// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package genscenario

import (
	"fmt"

	"github.com/gardener/scaling-advisor/tools/cmd/scadctl/cmd"

	"github.com/spf13/cobra"
)

// genscenarioCmd represents the genscenario command
var genscenarioCmd = &cobra.Command{
	Use:   "genscenario <cluster-manager>",
	Short: "Generate scaling data for the given cluster-manager",
	Long: `genscenario generates scaling data for the given cluster-manager.
Generate scaling scenario(s) for the gardener cluster identified by the given gardener landscape, gardener project
and gardener shoot name and write the scenario(s) to the scenario-dir.
	 genscenario gardener -l <landscape> -p <project> -t <shoot-name> -d <scenario-dir>
`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("genscenario called")
	},
}

func init() {
	cmd.RootCmd.AddCommand(genscenarioCmd)
}
