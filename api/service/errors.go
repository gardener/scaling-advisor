// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
)

var (
	// ErrInitFailed is a sentinel error indicating that the scaling-advisor service failed to initialize.
	ErrInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ProgramName)
	// ErrStartFailed is a sentinel error indicating that the scaling-advisor service failed to start.
	ErrStartFailed = fmt.Errorf(commonerrors.FmtStartFailed, ProgramName)
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
	// ErrComputeNodeScore is a sentinel error indicating that the NodeScorer failed to compute a score
	ErrComputeNodeScore = errors.New("failed to compute node score")
	// ErrNoWinningNodeScore is a sentinel error indicating that there is no winning NodeScore
	ErrNoWinningNodeScore = errors.New("no winning node score")
	// ErrSelectNodeScore is a sentinel error indicating that the NodeScoreSelector failed to select a score
	ErrSelectNodeScore = errors.New("failed to select node score")
	// ErrParseSchedulerConfig is a sentinel error indicating that the service failed to parse the scheduler configuration.
	ErrParseSchedulerConfig = errors.New("failed to parse scheduler configuration")
	// ErrLoadSchedulerConfig is a sentinel error indicating that the service failed to load the scheduler configuration.
	ErrLoadSchedulerConfig = errors.New("failed to load scheduler configuration")
	// ErrLaunchScheduler is a sentinel error indicating that the service failed to launch the scheduler.
	ErrLaunchScheduler = errors.New("failed to launch scheduler")
	// ErrNoUnscheduledPods is a sentinel error indicating that the service was wrongly invoked with no unscheduled pods.
	ErrNoUnscheduledPods = errors.New("no unscheduled pods")
	// ErrNoScalingAdvice is a sentinel error indicating that no scaling advice was generated.
	ErrNoScalingAdvice = errors.New("no scaling advice")
	// ErrUnsupportedNodeScoringStrategy is a sentinel error indicating an unsupported node scoring strategy was specified.
	ErrUnsupportedNodeScoringStrategy = errors.New("unsupported node scoring strategy")
	// ErrUnsupportedCloudProvider is a sentinel error indicating an unsupported cloud provider was specified.
	ErrUnsupportedCloudProvider = errors.New("unsupported cloud provider")
	// ErrLoadInstanceTypeInfo is a sentinel error indicating that instance type information could not be loaded.
	ErrLoadInstanceTypeInfo = errors.New("cannot load provider instance type info")
	// ErrMissingRequiredLabel is a sentinel error indicating that a required label is missing from a resource.
	ErrMissingRequiredLabel = errors.New("missing required label")
)

// AsGenerateError wraps an error with scaling advice request context information.
func AsGenerateError(id string, correlationID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ErrGenScalingAdvice, err) {
		return err
	}
	return fmt.Errorf("%w: could not process request with ID %q, CorrelationID %q: %w", ErrGenScalingAdvice, id, correlationID, err)
}
