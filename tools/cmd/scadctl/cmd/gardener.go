package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

type ShootCoordinate struct {
	Landscape string
	Project   string
	Shoot     string
}

var scenarioDir string
var shootCoords ShootCoordinate

// gardenerCmd represents the gardener sub-command for generating scaling scenario(s) for a gardener cluster.
var gardenerCmd = &cobra.Command{
	Use:   "gardener <scenario-dir>",
	Short: "generate scaling scenarios into <scenario-dir> for the gardener cluster manager",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("gardener called")
	},
}

func init() {
	genscenarioCmd.AddCommand(gardenerCmd)
	genscenarioCmd.Flags().StringVarP(
		&shootCoords.Landscape,
		"landscape", "l",
		"",
		"gardener landscape name",
	)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// gardenerCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// gardenerCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
