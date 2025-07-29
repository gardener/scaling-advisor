// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultLeaderElectionResourceLock = "leases"
	defaultLeaderElectionResourceName = "scalingadvisor-operator-leader-election"
	defaultServerPort                 = 8085
	defaultHealthProbePort            = 8086
	defaultMetricsPort                = 8087
	defaultProfilePort                = 8088
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
	if leaderElectionConfig.LeaseDuration.Duration == 0 {
		leaderElectionConfig.LeaseDuration = metav1.Duration{Duration: 15 * time.Second}
	}
	if leaderElectionConfig.RenewDeadline.Duration == 0 {
		leaderElectionConfig.RenewDeadline = metav1.Duration{Duration: 10 * time.Second}
	}
	if leaderElectionConfig.RetryPeriod.Duration == 0 {
		leaderElectionConfig.RetryPeriod = metav1.Duration{Duration: 2 * time.Second}
	}
	if leaderElectionConfig.ResourceLock == "" {
		leaderElectionConfig.ResourceLock = defaultLeaderElectionResourceLock
	}
	if leaderElectionConfig.ResourceName == "" {
		leaderElectionConfig.ResourceName = defaultLeaderElectionResourceName
	}
}

// SetDefaults_HealthProbes sets the defaults for health probes.
func SetDefaults_HealthProbes(healthProbesConfig *commontypes.HostPort) {
	if healthProbesConfig.Port == 0 {
		healthProbesConfig.Port = defaultHealthProbePort
	}
}

// SetDefaults_Metrics sets the defaults for metrics server configuration.
func SetDefaults_Metrics(metricsConfig *commontypes.HostPort) {
	if metricsConfig.Port == 0 {
		metricsConfig.Port = defaultMetricsPort
	}
}

// SetDefaults_Profiling sets the defaults for profiling.
func SetDefaults_Profiling(profilingConfig *commontypes.HostPort) {
	if profilingConfig.Port == 0 {
		profilingConfig.Port = defaultProfilePort
	}
}

// SetDefaults_ServerConfig sets the default for Server configuration.
func SetDefaults_ServerConfig(serverCfg *commontypes.ServerConfig) {
	if serverCfg.Port == 0 {
		serverCfg.Port = defaultServerPort
	}
	if serverCfg.GracefulShutdownTimeout.Duration == 0 {
		serverCfg.GracefulShutdownTimeout = metav1.Duration{Duration: 5 * time.Second}
	}
}
