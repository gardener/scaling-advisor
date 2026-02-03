// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"errors"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
)

var (
	// ErrGenScalingPlan is a high-level sentinel error indicating that the ScalingPlanner could not produce a scaling plan
	ErrGenScalingPlan = errors.New("cannot generate scaling plan")
	// ErrCreateSimulator is a sentinel error indicating that the planner failed to create a simulator.
	ErrCreateSimulator = errors.New("failed to create simulator")
	// ErrCreateSimulation is a sentinel error indicating that the planner failed to create a scaling simulation
	ErrCreateSimulation = errors.New("failed to create simulation")
	// ErrRunSimulation is a sentinel error indicating that a specific scaling simulation failed
	ErrRunSimulation = errors.New("failed to run simulation")
	// ErrRunSimulationGroup is a sentinel error indicating that a scaling simulation group failed
	ErrRunSimulationGroup = errors.New("failed to run simulation group")
	// ErrComputeNodeScore is a sentinel error indicating that the NodeScorer failed to compute a score
	ErrComputeNodeScore = errors.New("failed to compute node score")
	// ErrNoWinningNodeScore is a sentinel error indicating that there is no winning NodeScore
	ErrNoWinningNodeScore = errors.New("no winning node score")
	// ErrSelectNodeScore is a sentinel error indicating that the NodeScoreSelector failed to select a score
	ErrSelectNodeScore = errors.New("failed to select node score")
	// ErrParseSchedulerConfig is a sentinel error indicating that the planner failed to parse the kube-scheduler configuration.
	ErrParseSchedulerConfig = errors.New("failed to parse kube-scheduler configuration")
	// ErrLoadSchedulerConfig is a sentinel error indicating that the planner failed to load the kube-scheduler configuration.
	ErrLoadSchedulerConfig = errors.New("failed to load kube-scheduler configuration")
	// ErrLaunchScheduler is a sentinel error indicating that the planner failed to launch the kube-scheduler.
	ErrLaunchScheduler = errors.New("failed to launch kube-scheduler")
	// ErrNoUnscheduledPods is a sentinel error indicating that the planner was wrongly invoked with no unscheduled pods.
	ErrNoUnscheduledPods = errors.New("no unscheduled pods")
	// ErrNoScaleOutPlan is a sentinel error indicating that no ScaleOutPlan was generated.
	ErrNoScaleOutPlan = errors.New("no scale-out plan")
	// ErrCreateNodeScorer is a sentinel error indicating that the planner failed to create a NodeScorer.
	ErrCreateNodeScorer = errors.New("failed to create node scorer")
	// ErrInvalidScalingConstraint is a sentinel error indicating that the provided scaling constraint is invalid.
	ErrInvalidScalingConstraint = errors.New("invalid scaling constraint")
	// ErrUnsupportedSimulatorStrategy is a sentinel error indicating that an unsupported simulator strategy was specified.
	ErrUnsupportedSimulatorStrategy = errors.New("unsupported simulator strategy")
	// ErrInvalidRequest is a sentinel error indicating that the scaling planner request is invalid.
	ErrInvalidRequest = errors.New("invalid planner request")

	// ErrServiceInitFailed is a sentinel error indicating that the ScalingPlannerService failed to initialize.
	ErrServiceInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ServiceName)
	// ErrStartFailed is a sentinel error indicating that the  ScalingPlannerService failed to start.
	ErrStartFailed = fmt.Errorf(commonerrors.FmtStartFailed, ServiceName)
)

// AsGenError wraps the given error with the high-level sentinel error ErrGenScalingPlan and message mentioning the request id and correlationID.
func AsGenError(id string, correlationID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ErrGenScalingPlan, err) {
		return err
	}
	return fmt.Errorf("%w: could not process request with id %q, correlationID %q: %w", ErrGenScalingPlan, id, correlationID, err)
}
