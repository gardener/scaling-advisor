// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/gardener/scaling-advisor/minkapi/cli"
)

func main() {
	app, exitCode, err := cli.LaunchApp(context.Background())
	if err != nil {
		os.Exit(exitCode)
	}
	defer app.Cancel()

	<-app.Ctx.Done()
	exitCode = cli.ShutdownApp(&app)
	os.Exit(exitCode)
}
