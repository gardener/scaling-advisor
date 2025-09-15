// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/gardener/scaling-advisor/minkapi/server"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/go-logr/logr"
)

func main() {
	app, exitCode := server.LaunchApp(context.Background())
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	defer app.Cancel()

	log := logr.FromContextOrDiscard(app.Ctx)

	// Wait for a signal
	<-app.Ctx.Done()
	log.Info("Received shutdown signal, initiating graceful shutdown")

	exitCode = server.ShutdownApp(&app)
	os.Exit(exitCode)
}
