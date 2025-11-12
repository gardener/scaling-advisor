/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"github.com/gardener/scaling-advisor/tools/cmd/scadctl/cmd"

	_ "github.com/gardener/scaling-advisor/tools/cmd/scadctl/cmd/genscenario"
)

func main() {
	cmd.Execute()
}
