// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/gardener/scaling-advisor/operator/internal/controller/scalingconstraints"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	configv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"
	corev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	ctrlmetricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// CreateManagerAndRegisterControllers creates a controller manager and registers all controllers.
func CreateManagerAndRegisterControllers(log logr.Logger, saCfg *configv1alpha1.OperatorConfig) (ctrl.Manager, error) {
	mgrOpts, err := createManagerOptions(log, saCfg)
	if err != nil {
		return nil, err
	}
	mgr, err := ctrl.NewManager(getRestConfig(saCfg), mgrOpts)
	if err != nil {
		return nil, err
	}
	if err = registerControllers(mgr, saCfg.Controllers); err != nil {
		return nil, err
	}
	return mgr, nil
}

func createManagerOptions(log logr.Logger, saCfg *configv1alpha1.OperatorConfig) (ctrl.Options, error) {
	scheme, err := createScalingAdvisorScheme()
	if err != nil {
		return ctrl.Options{}, err
	}
	opts := ctrl.Options{
		Scheme:                  scheme,
		GracefulShutdownTimeout: ptr.To(commonconstants.DefaultGracefulShutdownTimeout),
		Logger:                  log,
		Metrics: ctrlmetricsserver.Options{
			BindAddress: saCfg.Server.MetricsBindAddress,
		},
		LeaderElection:                saCfg.LeaderElection.Enabled,
		LeaderElectionID:              saCfg.LeaderElection.ResourceName,
		LeaderElectionResourceLock:    saCfg.LeaderElection.ResourceLock,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &saCfg.LeaderElection.LeaseDuration.Duration,
		RenewDeadline:                 &saCfg.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:                   &saCfg.LeaderElection.RetryPeriod.Duration,
		Controller: ctrlconfig.Controller{
			RecoverPanic: ptr.To(true),
		},
	}
	if saCfg.Server.ProfilingEnabled {
		opts.PprofBindAddress = saCfg.Server.ProfilingBindAddress
	}
	return opts, nil
}

func getRestConfig(operatorCfg *configv1alpha1.OperatorConfig) *rest.Config {
	restCfg := ctrl.GetConfigOrDie()
	if operatorCfg != nil {
		restCfg.Burst = operatorCfg.ClientConnection.Burst
		restCfg.QPS = operatorCfg.ClientConnection.QPS
		restCfg.AcceptContentTypes = operatorCfg.ClientConnection.AcceptContentTypes
		restCfg.ContentType = operatorCfg.ClientConnection.ContentType
	}
	return restCfg
}

func createScalingAdvisorScheme() (*runtime.Scheme, error) {
	localSchemeBuilder := runtime.NewSchemeBuilder(
		k8sscheme.AddToScheme,
		configv1alpha1.AddToScheme,
		corev1alpha1.AddToScheme,
	)
	scheme := runtime.NewScheme()
	if err := localSchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func registerControllers(mgr ctrl.Manager, controllersConfig configv1alpha1.ControllersConfig) error {
	scalingConstraintsController := scalingconstraints.NewReconciler(mgr, controllersConfig.ScalingConstraints)
	return scalingConstraintsController.SetupWithManager(mgr)
}
