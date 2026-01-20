// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strings"
	"time"

	"github.com/gardener/scaling-advisor/api/common/constants"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultLeaderElectionResourceLock = "leases"
	defaultLeaderElectionResourceName = "scalingadvisor-operator-leader-election"
	defaultConcurrentSyncs            = 2
)

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
		serverConfig.HealthProbeBindAddress = constants.DefaultOperatorHealthProbeBindAddress
	}
	if strings.TrimSpace(serverConfig.MetricsBindAddress) == "" {
		serverConfig.MetricsBindAddress = constants.DefaultOperatorMetricsBindAddress
	}
	if serverConfig.ProfilingEnabled {
		if strings.TrimSpace(serverConfig.ProfilingBindAddress) == "" {
			serverConfig.ProfilingBindAddress = constants.DefaultOperatorProfilingBindAddress
		}
	}
}

// SetDefaults_ScalingConstraintsControllerConfiguration sets defaults for the ScalingConstraintsControllerConfig.
func SetDefaults_ScalingConstraintsControllerConfiguration(scalingConstraintsConfig *ScalingConstraintsControllerConfig) {
	if scalingConstraintsConfig.ConcurrentSyncs <= 0 {
		scalingConstraintsConfig.ConcurrentSyncs = defaultConcurrentSyncs
	}
}
