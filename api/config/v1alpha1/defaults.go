// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strings"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultLeaderElectionResourceLock      = "leases"
	defaultLeaderElectionResourceName      = "scalingadvisor-leader-election"
	defaultLeaderElectionResourceNamespace = "kube-system"
	defaultConcurrentSyncs                 = 2
)

// SetDefaults_OperatorConfig sets defaults for all fields of the OperatorConfig.
func SetDefaults_OperatorConfig(config *OperatorConfig) {
	SetDefaults_ScalingAdvisorServerConfiguration(&config.Server)
	SetDefaults_ClientConnectionConfiguration(&config.ClientConnection)
	SetDefaults_LeaderElectionConfiguration(&config.LeaderElection)
	SetDefaults_ScalingConstraintsControllerConfiguration(&config.Controllers.ScalingConstraints)
}

// SetDefaults_ClientConnectionConfiguration sets defaults for the k8s client connection.
func SetDefaults_ClientConnectionConfiguration(clientConnConfig *ClientConnectionConfig) {
	if clientConnConfig.QPS == 0.0 {
		clientConnConfig.QPS = 100.0
	}
	if clientConnConfig.Burst == 0 {
		clientConnConfig.Burst = 120
	}
}

// SetDefaults_LeaderElectionConfiguration sets defaults for the leader election of the scalingadvisor operator.
func SetDefaults_LeaderElectionConfiguration(leaderElectionConfig *LeaderElectionConfig) {
	zero := metav1.Duration{}
	if leaderElectionConfig.LeaseDuration == zero {
		leaderElectionConfig.LeaseDuration = metav1.Duration{Duration: 15 * time.Second}
	}
	if leaderElectionConfig.RenewDeadline == zero {
		leaderElectionConfig.RenewDeadline = metav1.Duration{Duration: 10 * time.Second}
	}
	if leaderElectionConfig.RetryPeriod == zero {
		leaderElectionConfig.RetryPeriod = metav1.Duration{Duration: 2 * time.Second}
	}
	if leaderElectionConfig.ResourceNamespace == "" {
		leaderElectionConfig.ResourceNamespace = defaultLeaderElectionResourceNamespace
	}
	if leaderElectionConfig.ResourceLock == "" {
		leaderElectionConfig.ResourceLock = defaultLeaderElectionResourceLock
	}
	if leaderElectionConfig.ResourceName == "" {
		leaderElectionConfig.ResourceName = defaultLeaderElectionResourceName
	}
}

// SetDefaults_ScalingAdvisorServerConfiguration sets defaults for ScalingAdvisorServerConfig.
func SetDefaults_ScalingAdvisorServerConfiguration(serverConfig *ScalingAdvisorServerConfig) {
	if strings.TrimSpace(serverConfig.HealthProbeBindAddress) == "" {
		serverConfig.HealthProbeBindAddress = apicommon.DefaultOperatorHealthProbeBindAddress
	}
	if strings.TrimSpace(serverConfig.MetricsBindAddress) == "" {
		serverConfig.MetricsBindAddress = apicommon.DefaultOperatorMetricsBindAddress
	}
	if serverConfig.ProfilingEnabled {
		if strings.TrimSpace(serverConfig.ProfilingBindAddress) == "" {
			serverConfig.ProfilingBindAddress = apicommon.DefaultOperatorProfilingBindAddress
		}
	}
}

// SetDefaults_ScalingConstraintsControllerConfiguration sets defaults for the ScalingConstraintsControllerConfig.
func SetDefaults_ScalingConstraintsControllerConfiguration(scalingConstraintsConfig *ScalingConstraintsControllerConfig) {
	if scalingConstraintsConfig.ConcurrentSyncs <= 0 {
		scalingConstraintsConfig.ConcurrentSyncs = defaultConcurrentSyncs
	}
}
