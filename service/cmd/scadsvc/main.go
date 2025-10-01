// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/service/internal/service"
	"github.com/go-logr/logr"
	"os"
)

func main() {
	app, exitCode := service.LaunchApp(context.Background())
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	defer app.Cancel()

	log := logr.FromContextOrDiscard(app.Ctx)

	// Wait for a signal
	<-app.Ctx.Done()
	log.Info("Received shutdown signal, initiating graceful shutdown")

	exitCode = service.ShutdownApp(&app)
	os.Exit(exitCode)
}
