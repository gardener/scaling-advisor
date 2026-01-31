// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/gardener/scaling-advisor/operator/cmd/scalingadvisor/cli"
	"github.com/gardener/scaling-advisor/operator/internal/controller"

	"github.com/gardener/scaling-advisor/api/common/constants"
	commoncli "github.com/gardener/scaling-advisor/common/cliutil"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	// Set up logr with klog backend using NewKlogr
	log := klog.NewKlogr()

	launchOpts, err := cli.ParseLaunchOptions(os.Args[1:])
	if err != nil {
		commoncli.HandleErrorAndExit(err)
	}
	if launchOpts.Version {
		commoncli.PrintVersion(constants.OperatorName)
		os.Exit(commoncli.ExitSuccess)
	}

	operatorConfig, err := launchOpts.LoadAndValidateOperatorConfig()
	if err != nil {
		commoncli.HandleErrorAndExit(err)
	}

	log.Info("loaded configuration", "operatorConfig", operatorConfig)

	mgr, err := controller.CreateManagerAndRegisterControllers(log, operatorConfig)
	if err != nil {
		commoncli.HandleErrorAndExit(err)
	}

	ctx := ctrl.SetupSignalHandler()
	if err := mgr.Start(ctx); err != nil {
		commoncli.HandleErrorAndExit(err)
	}
}
