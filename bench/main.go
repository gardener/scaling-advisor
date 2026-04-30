// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	bench "github.com/gardener/scaling-advisor/bench/cmd"

	_ "github.com/gardener/scaling-advisor/bench/cmd/exec"
	_ "github.com/gardener/scaling-advisor/bench/cmd/setup"
)

func main() {
	bench.Execute()
}
