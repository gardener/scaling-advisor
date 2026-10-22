// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"
	"time"

	configv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"
	"github.com/gardener/scaling-advisor/api/config/v1alpha1/validation"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateServerConfiguration(t *testing.T) {
	cases := []struct {
		name       string
		config     configv1alpha1.ScalingAdvisorServerConfig
		wantFields []string
	}{
		{
			name:   "valid with all addresses set",
			config: validServerConfig(),
		},
		{
			name: "missing healthProbeBindAddress",
			config: configv1alpha1.ScalingAdvisorServerConfig{
				MetricsBindAddress: ":8082",
			},
			wantFields: []string{"server.healthProbeBindAddress"},
		},
		{
			name: "missing metricsBindAddress",
			config: configv1alpha1.ScalingAdvisorServerConfig{
				HealthProbeBindAddress: ":8081",
			},
			wantFields: []string{"server.metricsBindAddress"},
		},
		{
			name: "profilingEnabled but no profilingBindAddress",
			config: configv1alpha1.ScalingAdvisorServerConfig{
				HealthProbeBindAddress: ":8081",
				MetricsBindAddress:     ":8082",
				ProfilingEnabled:       true,
			},
			wantFields: []string{"server.profilingBindAddress"},
		},
		{
			name: "profilingEnabled with profilingBindAddress set",
			config: configv1alpha1.ScalingAdvisorServerConfig{
				HealthProbeBindAddress: ":8081",
				MetricsBindAddress:     ":8082",
				ProfilingEnabled:       true,
				ProfilingBindAddress:   ":8083",
			},
		},
		{
			name:       "both required addresses missing",
			config:     configv1alpha1.ScalingAdvisorServerConfig{},
			wantFields: []string{"server.healthProbeBindAddress", "server.metricsBindAddress"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validation.ValidateScalingAdvisorConfiguration(configWithServer(tc.config))
			assertFieldErrors(t, errs, tc.wantFields)
		})
	}
}

func TestValidateClientConnectionConfiguration(t *testing.T) {
	cases := []struct {
		name       string
		wantFields []string
		config     configv1alpha1.ClientConnectionConfig
	}{
		{
			name:   "valid defaults",
			config: configv1alpha1.ClientConnectionConfig{QPS: 100, Burst: 120},
		},
		{
			name:       "negative QPS",
			config:     configv1alpha1.ClientConnectionConfig{QPS: -1, Burst: 120},
			wantFields: []string{"clientConnection.qps"},
		},
		{
			name:       "negative Burst",
			config:     configv1alpha1.ClientConnectionConfig{QPS: 100, Burst: -1},
			wantFields: []string{"clientConnection.burst"},
		},
		{
			name:       "both negative",
			config:     configv1alpha1.ClientConnectionConfig{QPS: -1, Burst: -1},
			wantFields: []string{"clientConnection.qps", "clientConnection.burst"},
		},
		{
			name:   "zero QPS and Burst are valid",
			config: configv1alpha1.ClientConnectionConfig{QPS: 0, Burst: 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validation.ValidateScalingAdvisorConfiguration(configWithClientConnection(tc.config))
			assertFieldErrors(t, errs, tc.wantFields)
		})
	}
}

func TestValidateLeaderElectionConfiguration(t *testing.T) {
	cases := []struct {
		name       string
		wantFields []string
		config     configv1alpha1.LeaderElectionConfig
	}{
		{
			name:   "disabled - no validation",
			config: configv1alpha1.LeaderElectionConfig{Enabled: false},
		},
		{
			name:   "valid enabled config",
			config: validLeaderElectionConfig(),
		},
		{
			name: "zero leaseDuration",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.LeaseDuration = metav1.Duration{}
				return c
			}(),
			wantFields: []string{"leaderElection.leaseDuration"},
		},
		{
			name: "zero renewDeadline",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.RenewDeadline = metav1.Duration{}
				return c
			}(),
			wantFields: []string{"leaderElection.renewDeadline"},
		},
		{
			name: "zero retryPeriod",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.RetryPeriod = metav1.Duration{}
				return c
			}(),
			wantFields: []string{"leaderElection.retryPeriod"},
		},
		{
			name: "leaseDuration not greater than renewDeadline",
			config: configv1alpha1.LeaderElectionConfig{
				Enabled:           true,
				LeaseDuration:     metav1.Duration{Duration: 10 * time.Second},
				RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
				ResourceLock:      "leases",
				ResourceName:      "leader",
				ResourceNamespace: "kube-system",
			},
			wantFields: []string{"leaderElection.leaseDuration"},
		},
		{
			name: "retryPeriod not less than renewDeadline",
			config: configv1alpha1.LeaderElectionConfig{
				Enabled:           true,
				LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
				RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:       metav1.Duration{Duration: 10 * time.Second},
				ResourceLock:      "leases",
				ResourceName:      "leader",
				ResourceNamespace: "kube-system",
			},
			wantFields: []string{"leaderElection.retryPeriod"},
		},
		{
			name: "zero leaseDuration and renewDeadline - no relational error",
			config: configv1alpha1.LeaderElectionConfig{
				Enabled:           true,
				LeaseDuration:     metav1.Duration{},
				RenewDeadline:     metav1.Duration{},
				RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
				ResourceLock:      "leases",
				ResourceName:      "leader",
				ResourceNamespace: "kube-system",
			},
			// only individual duration errors, not the relational leaseDuration error
			wantFields: []string{"leaderElection.leaseDuration", "leaderElection.renewDeadline"},
		},
		{
			name: "missing resourceLock",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.ResourceLock = ""
				return c
			}(),
			wantFields: []string{"leaderElection.resourceLock"},
		},
		{
			name: "missing resourceNamespace",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.ResourceNamespace = ""
				return c
			}(),
			wantFields: []string{"leaderElection.resourceNamespace"},
		},
		{
			name: "missing resourceName",
			config: func() configv1alpha1.LeaderElectionConfig {
				c := validLeaderElectionConfig()
				c.ResourceName = ""
				return c
			}(),
			wantFields: []string{"leaderElection.resourceName"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validation.ValidateScalingAdvisorConfiguration(configWithLeaderElection(tc.config))
			assertFieldErrors(t, errs, tc.wantFields)
		})
	}
}

func TestValidateControllersConfig(t *testing.T) {
	cases := []struct {
		name       string
		wantFields []string
		config     configv1alpha1.ControllersConfig
	}{
		{
			name:   "valid",
			config: configv1alpha1.ControllersConfig{ScalingConstraints: configv1alpha1.ScalingConstraintsControllerConfig{ConcurrentSyncs: 2}},
		},
		{
			name:       "zero concurrentSyncs",
			config:     configv1alpha1.ControllersConfig{ScalingConstraints: configv1alpha1.ScalingConstraintsControllerConfig{ConcurrentSyncs: 0}},
			wantFields: []string{"controllers.scalingConstraints.concurrentSyncs"},
		},
		{
			name:       "negative concurrentSyncs",
			config:     configv1alpha1.ControllersConfig{ScalingConstraints: configv1alpha1.ScalingConstraintsControllerConfig{ConcurrentSyncs: -1}},
			wantFields: []string{"controllers.scalingConstraints.concurrentSyncs"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validation.ValidateScalingAdvisorConfiguration(configWithControllers(tc.config))
			assertFieldErrors(t, errs, tc.wantFields)
		})
	}
}

// --- helpers ---

func validConfig() *configv1alpha1.OperatorConfig {
	return &configv1alpha1.OperatorConfig{
		Server:           validServerConfig(),
		ClientConnection: configv1alpha1.ClientConnectionConfig{QPS: 100, Burst: 120},
		LeaderElection:   validLeaderElectionConfig(),
		Controllers:      configv1alpha1.ControllersConfig{ScalingConstraints: configv1alpha1.ScalingConstraintsControllerConfig{ConcurrentSyncs: 2}},
	}
}

func configWithServer(s configv1alpha1.ScalingAdvisorServerConfig) *configv1alpha1.OperatorConfig {
	c := validConfig()
	c.Server = s
	return c
}

func configWithClientConnection(cc configv1alpha1.ClientConnectionConfig) *configv1alpha1.OperatorConfig {
	c := validConfig()
	c.ClientConnection = cc
	return c
}

func configWithLeaderElection(le configv1alpha1.LeaderElectionConfig) *configv1alpha1.OperatorConfig {
	c := validConfig()
	c.LeaderElection = le
	return c
}

func configWithControllers(ctrl configv1alpha1.ControllersConfig) *configv1alpha1.OperatorConfig {
	c := validConfig()
	c.Controllers = ctrl
	return c
}

func validServerConfig() configv1alpha1.ScalingAdvisorServerConfig {
	return configv1alpha1.ScalingAdvisorServerConfig{
		HealthProbeBindAddress: ":8081",
		MetricsBindAddress:     ":8082",
	}
}

func validLeaderElectionConfig() configv1alpha1.LeaderElectionConfig {
	return configv1alpha1.LeaderElectionConfig{
		Enabled:           true,
		LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
		RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
		RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
		ResourceLock:      "leases",
		ResourceName:      "scalingadvisor-operator-leader-election",
		ResourceNamespace: "kube-system",
	}
}

func assertFieldErrors(t *testing.T, errs field.ErrorList, wantFields []string) {
	t.Helper()
	got := make(map[string]bool, len(errs))
	for _, e := range errs {
		got[e.Field] = true
	}
	want := make(map[string]bool, len(wantFields))
	for _, f := range wantFields {
		want[f] = true
	}
	for f := range want {
		if !got[f] {
			t.Errorf("expected error on field %q but got none; all errors: %v", f, errs)
		}
	}
	for f := range got {
		if !want[f] {
			t.Errorf("unexpected error on field %q", f)
		}
	}
}
