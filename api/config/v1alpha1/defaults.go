// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	"github.com/gardener/scaling-advisor/api/common/constants"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	defaultLeaderElectionResourceLock = "leases"
	defaultLeaderElectionResourceName = "scalingadvisor-operator-leader-election"
)

// SetDefaults_ClientConnectionConfiguration sets defaults for the k8s client connection.
func SetDefaults_ClientConnectionConfiguration(clientConnConfig *ClientConnectionConfiguration) {
	if clientConnConfig.QPS == 0.0 {
		clientConnConfig.QPS = 100.0
	}
	if clientConnConfig.Burst == 0 {
		clientConnConfig.Burst = 120
	}
}

// SetDefaults_LeaderElectionConfiguration sets defaults for the leader election of the scalingadvisor operator.
func SetDefaults_LeaderElectionConfiguration(leaderElectionConfig *LeaderElectionConfiguration) {
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

// SetDefaults_ScalingAdvisorServerConfiguration sets defaults for ScalingAdvisorServerConfiguration.
func SetDefaults_ScalingAdvisorServerConfiguration(serverConfig *ScalingAdvisorServerConfiguration) {
	if serverConfig.Port == 0 {
		serverConfig.Port = constants.DefaultOperatorServerPort
	}
	if serverConfig.GracefulShutdownTimeout.Duration == 0 {
		serverConfig.GracefulShutdownTimeout = metav1.Duration{Duration: 5 * time.Second}
	}

	if serverConfig.HealthProbes.Port == 0 {
		serverConfig.HealthProbes.Port = constants.DefaultOperatorHealthProbePort
	}

	if serverConfig.Metrics.Port == 0 {
		serverConfig.Metrics.Port = constants.DefaultOperatorMetricsPort
	}

	if serverConfig.Profiling.Port == 0 {
		serverConfig.Profiling.Port = constants.DefaultOperatorProfilingPort
	}
}

// SetDefaults_ScalingConstraintsControllerConfiguration sets defaults for the ScalingConstraintsControllerConfiguration.
func SetDefaults_ScalingConstraintsControllerConfiguration(scalingConstraintsConfig *ScalingConstraintsControllerConfiguration) {
	if scalingConstraintsConfig.ConcurrentSyncs == nil {
		scalingConstraintsConfig.ConcurrentSyncs = ptr.To(1)
	}
}
