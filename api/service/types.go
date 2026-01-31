// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
)

const (
	// ProgramName is the program name for the scaling advisor service.
	ProgramName = "scadsvc"
)

// App represents an application process that wraps a ScalingAdvisorService, an application context and application cancel func.
// `main` entry-point functions that embed scadsvc are expected to construct a new App instance via cli.LaunchApp and shutdown applications via cli.ShutdownApp
type App struct {
	// Service is the scaling advisor service instance.
	Service ScalingAdvisorService
	// Ctx is the application context.
	Ctx context.Context
	// Cancel is the context cancellation function.
	Cancel context.CancelFunc
}

// ScalingAdvisorService is the high-level facade for the scaling advisor service.
type ScalingAdvisorService interface {
	commontypes.Service
	// GenerateAdvice begins generating scaling advice for the given request.
	//
	// It returns a channel of ScalingAdviceResult values. The channel will be closed
	// when advice generation is completed, an error has occurred, or the context is canceled or timed-out.
	//
	// The caller must consume all events from the channel until it is closed to
	// avoid leaking goroutines inside the service.
	//
	// The provided context can be used to cancel generation prematurely. In this
	// case, the channel will be closed without further events.
	//
	// Usage:
	//
	//	results := svc.GenerateAdvice(ctx, req)
	//	for r := range results {
	//	    if r.Err != nil {
	//	        log.Printf("advice generation failed: %v", r.Err)
	//	        break
	//	    }
	//	    process(r.Response)
	//	}
	GenerateAdvice(ctx context.Context, req planner.ScalingAdviceRequest) <-chan planner.ScalingAdviceResult
}

// ScalingAdvisorServiceConfig holds the configuration for the scaling advisor planner.
type ScalingAdvisorServiceConfig struct {
	// CloudProvider is the cloud provider for which the scaling advisor planner is initialized.
	CloudProvider commontypes.CloudProvider
	// TraceLogBaseDir is the base directory for storing trace log files used by the scaling advisor planner.
	TraceLogBaseDir string
	// ServerConfig holds the server configuration for the scaling advisor planner.
	ServerConfig commontypes.ServerConfig
	// MinKAPIConfig holds the configuration for the MinKAPI server used by the scaling advisor planner.
	MinKAPIConfig minkapi.Config
	// ClientConfig holds the client QPS and Burst settings for the scaling advisor planner.
	ClientConfig commontypes.QPSBurst
	// SimulatorConfig holds the configuration used by the internal simulator.
	SimulatorConfig planner.SimulatorConfig
}
