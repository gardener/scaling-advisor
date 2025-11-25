// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/gardener/scaling-advisor/tools/cmd/scadctl/cmd"

	_ "github.com/gardener/scaling-advisor/tools/cmd/scadctl/cmd/genscenario"
)

func main() {
	cmd.Execute()
}
