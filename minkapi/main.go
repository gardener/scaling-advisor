// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/minkapi/core"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

func main() {
	commoncli.PrintVersion(api.ProgramName)
	mainOpts, err := cli.ParseProgramFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		os.Exit(commoncli.ExitErrParseOpts)
	}
	// Set up logr with klog backend using NewKlogr
	log := klog.NewKlogr()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	service, err := core.NewInMemoryMinKAPI(logr.NewContext(ctx, log), mainOpts.MinKAPIConfig)
	if err != nil {
		log.Error(err, "failed to initialize InMemoryKAPI")
		return
	}
	// Start the service in a goroutine
	go func() {
		if err := service.Start(); err != nil {
			if errors.Is(err, api.ErrStartFailed) {
				log.Error(err, "failed to start service")
			} else {
				log.Error(err, fmt.Sprintf("%s start failed", api.ProgramName))
			}
			os.Exit(commoncli.ExitErrStart)
		}
	}()

	// Wait for a signal
	<-ctx.Done()
	log.Info("Received shutdown signal, initiating graceful shutdown")

	// Create a context with a 5-second timeout for shutdown
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Perform shutdown
	if err := service.Shutdown(shutDownCtx); err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", api.ProgramName))
		os.Exit(commoncli.ExitErrShutdown)
	}
	log.Info(fmt.Sprintf("%s shutdown gracefully.", api.ProgramName))
}
