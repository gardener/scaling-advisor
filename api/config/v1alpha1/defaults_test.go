// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/api/config/v1alpha1"
)

func TestSetDefaults_OperatorConfig(t *testing.T) {
	cfg := &v1alpha1.OperatorConfig{}
	v1alpha1.SetDefaults_OperatorConfig(cfg)

	// Server
	if cfg.Server.HealthProbeBindAddress == "" {
		t.Error("expected HealthProbeBindAddress to be defaulted")
	}
	if cfg.Server.MetricsBindAddress == "" {
		t.Error("expected MetricsBindAddress to be defaulted")
	}

	// ClientConnection
	if cfg.ClientConnection.QPS == 0 {
		t.Error("expected QPS to be defaulted")
	}
	if cfg.ClientConnection.Burst == 0 {
		t.Error("expected Burst to be defaulted")
	}

	// LeaderElection
	if cfg.LeaderElection.LeaseDuration.Duration == 0 {
		t.Error("expected LeaseDuration to be defaulted")
	}
	if cfg.LeaderElection.RenewDeadline.Duration == 0 {
		t.Error("expected RenewDeadline to be defaulted")
	}
	if cfg.LeaderElection.RetryPeriod.Duration == 0 {
		t.Error("expected RetryPeriod to be defaulted")
	}
	if cfg.LeaderElection.ResourceLock == "" {
		t.Error("expected ResourceLock to be defaulted")
	}
	if cfg.LeaderElection.ResourceName == "" {
		t.Error("expected ResourceName to be defaulted")
	}
	if cfg.LeaderElection.ResourceNamespace == "" {
		t.Error("expected ResourceNamespace to be defaulted")
	}

	// Controllers
	if cfg.Controllers.ScalingConstraints.ConcurrentSyncs == 0 {
		t.Error("expected ConcurrentSyncs to be defaulted")
	}
}

func TestSetDefaults_LeaderElectionConfiguration(t *testing.T) {
	t.Run("all fields defaulted on zero value", func(t *testing.T) {
		cfg := &v1alpha1.LeaderElectionConfig{}
		v1alpha1.SetDefaults_LeaderElectionConfiguration(cfg)

		if cfg.LeaseDuration.Duration != 15*time.Second {
			t.Errorf("LeaseDuration: got %v, want 15s", cfg.LeaseDuration.Duration)
		}
		if cfg.RenewDeadline.Duration != 10*time.Second {
			t.Errorf("RenewDeadline: got %v, want 10s", cfg.RenewDeadline.Duration)
		}
		if cfg.RetryPeriod.Duration != 2*time.Second {
			t.Errorf("RetryPeriod: got %v, want 2s", cfg.RetryPeriod.Duration)
		}
		if cfg.ResourceLock != "leases" {
			t.Errorf("ResourceLock: got %q, want %q", cfg.ResourceLock, "leases")
		}
		if cfg.ResourceName == "" {
			t.Error("expected ResourceName to be defaulted")
		}
		if cfg.ResourceNamespace != "kube-system" {
			t.Errorf("ResourceNamespace: got %q, want %q", cfg.ResourceNamespace, "kube-system")
		}
	})

	t.Run("existing values are not overwritten", func(t *testing.T) {
		cfg := &v1alpha1.LeaderElectionConfig{
			ResourceNamespace: "my-namespace",
			ResourceLock:      "configmaps",
		}
		v1alpha1.SetDefaults_LeaderElectionConfiguration(cfg)

		if cfg.ResourceNamespace != "my-namespace" {
			t.Errorf("ResourceNamespace should not be overwritten, got %q", cfg.ResourceNamespace)
		}
		if cfg.ResourceLock != "configmaps" {
			t.Errorf("ResourceLock should not be overwritten, got %q", cfg.ResourceLock)
		}
	})
}

func TestSetDefaults_ClientConnectionConfiguration(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := &v1alpha1.ClientConnectionConfig{}
		v1alpha1.SetDefaults_ClientConnectionConfiguration(cfg)
		if cfg.QPS != 100.0 {
			t.Errorf("QPS: got %v, want 100", cfg.QPS)
		}
		if cfg.Burst != 120 {
			t.Errorf("Burst: got %v, want 120", cfg.Burst)
		}
	})

	t.Run("existing values are not overwritten", func(t *testing.T) {
		cfg := &v1alpha1.ClientConnectionConfig{QPS: 50, Burst: 60}
		v1alpha1.SetDefaults_ClientConnectionConfiguration(cfg)
		if cfg.QPS != 50 {
			t.Errorf("QPS should not be overwritten, got %v", cfg.QPS)
		}
		if cfg.Burst != 60 {
			t.Errorf("Burst should not be overwritten, got %v", cfg.Burst)
		}
	})
}
