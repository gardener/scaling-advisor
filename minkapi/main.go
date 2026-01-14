// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/gardener/scaling-advisor/minkapi/cli"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
)

func main() {
	app, exitCode := cli.LaunchApp(context.Background())
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	defer app.Cancel()

	<-app.Ctx.Done()
	exitCode = cli.ShutdownApp(&app)
	os.Exit(exitCode)
}
