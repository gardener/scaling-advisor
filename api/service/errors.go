// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"fmt"
)

var (
	// ErrInitFailed is a sentinel error indicating that the scaling-advisor service failed to initialize.
	ErrInitFailed = fmt.Errorf("failed to initialize %s service", ServiceName)
	// ErrGenScalingAdvice is a sentinel error indicating that the service failed to generate scaling advice.
	ErrGenScalingAdvice = errors.New("failed to generate scaling advice")
	// ErrCreateSimulation is a sentinel error indicating that the service failed to create a scaling simulation
	ErrCreateSimulation = errors.New("failed to create simulation")
	// ErrRunSimulation is a sentinel error indicating that a specific scaling simulation failed
	ErrRunSimulation = errors.New("failed to run simulation")
	// ErrRunSimulationGroup is a sentinel error indicating that a scaling simulation group failed
	ErrRunSimulationGroup = errors.New("failed to run simulation group")

	// ErrSimulationTimeout is a sentinel error indicating that the simulation timed out
	ErrSimulationTimeout = errors.New("simulation timed out")
	//ErrComputeNodeScore is a sentinel error indicating that the NodeScorer failed to compute a score
	ErrComputeNodeScore = errors.New("failed to compute node score")
	// ErrNoWinningNodeScore is a sentinel error indicating that there is no winning NodeScore
	ErrNoWinningNodeScore = errors.New("no winning node score")
	//ErrSelectNodeScore is a sentinel error indicating that the NodeScoreSelector failed to select a score
	ErrSelectNodeScore = errors.New("failed to select node score")
	// ErrLoadSchedulerConfig is a sentinel error indicating that the service failed to load the scheduler configuration.
	ErrLoadSchedulerConfig = errors.New("failed to load scheduler configuration")
	// ErrLaunchScheduler is a sentinel error indicating that the service failed to launch the scheduler.
	ErrLaunchScheduler = errors.New("failed to launch scheduler")
	// ErrNoUnscheduledPods is a sentinel error indicating that the service was wrongly invoked with no unscheduled pods.
	ErrNoUnscheduledPods              = errors.New("no unscheduled pods")
	ErrNoScalingAdvice                = errors.New("no scaling advice")
	ErrUnsupportedNodeScoringStrategy = errors.New("unsupported node scoring strategy")
	ErrUnsupportedCloudProvider       = errors.New("unsupported cloud provider")

	ErrLoadInstanceTypeInfo = errors.New("cannot load provider instance type info")
	ErrMissingRequiredLabel = errors.New("missing required label")
)

func AsGenerateError(id string, correlationID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ErrGenScalingAdvice, err) {
		return err
	}
	return fmt.Errorf("%w: could not process request with ID %q, CorrelationID %q: %w", ErrGenScalingAdvice, id, correlationID, err)
}
