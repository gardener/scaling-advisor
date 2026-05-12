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
	// ErrCreatePlanner indicates that the planner coould not be created
	ErrCreatePlanner = errors.New("cannot create planner")
	// ErrPopulateRequestView indicates that the planner could not populate the request view
	ErrPopulateRequestView = errors.New("cannot populate request view")
	// ErrCreateSimulator indicates that the planner cannot create a simulator.
	ErrCreateSimulator = errors.New("cannot create simulator")
	// ErrCreateSimulation indicates that the planner cannot create a scaling simulation
	ErrCreateSimulation = errors.New("cannot create simulation")
	// ErrRunSimulation indicates that planner could not successfully run a specific scaling simulation
	ErrRunSimulation = errors.New("cannot run simulation")
	// ErrCreateSimNodes indicates that the simulator failed to create simulation node(s).
	ErrCreateSimNodes = errors.New("cannot create simulation node(s)")
	// ErrRunSimulationGroup indicates that the planner could not run a scaling simulation group.
	ErrRunSimulationGroup = errors.New("cannot run simulation group")

	// ErrBindClaimVolume indicates that a scaling simulation cannot bind PVC<->PV
	ErrBindClaimVolume = errors.New("cannot bind claim to volume")
	// ErrProvisionVolume indicates that a scaling simulation cannot dynamically provision a simulated PV
	ErrProvisionVolume = errors.New("cannot provision volume")
	// ErrComputeNodeScore indicates that the NodeScorer cannot compute a score
	ErrComputeNodeScore = errors.New("cannot compute node score")
	// ErrNoWinningNodeScore indicates that there is no winning NodeScore
	ErrNoWinningNodeScore = errors.New("no winning node score")
	// ErrSelectNodeScore indicates that the NodeScoreSelector cannot select a score
	ErrSelectNodeScore = errors.New("cannot select node score")
	// ErrParseSchedulerConfig indicates that the planner cannot parse the kube-scheduler configuration.
	ErrParseSchedulerConfig = errors.New("cannot parse kube-scheduler configuration")
	// ErrLoadSchedulerConfig indicates that the planner cannot load the kube-scheduler configuration.
	ErrLoadSchedulerConfig = errors.New("cannot load kube-scheduler configuration")
	// ErrLaunchScheduler indicates that the planner cannot launch the kube-scheduler.
	ErrLaunchScheduler = errors.New("cannot launch kube-scheduler")
	// ErrNoUnscheduledPods indicates that the planner was wrongly invoked with no unscheduled pods.
	ErrNoUnscheduledPods = errors.New("no unscheduled pods")
	// ErrNoScaleOutPlan indicates that no ScaleOutPlan was generated.
	ErrNoScaleOutPlan = errors.New("no scale-out plan")
	// ErrCreateNodeScorer indicates that the planner cannot create a NodeScorer.
	ErrCreateNodeScorer = errors.New("cannot create node scorer")
	// ErrInvalidScalingConstraint indicates that the provided scaling constraint is invalid.
	ErrInvalidScalingConstraint = errors.New("invalid scaling constraint")
	// ErrUnsupportedSimulatorStrategy indicates that an unsupported simulator strategy was specified.
	ErrUnsupportedSimulatorStrategy = errors.New("unsupported simulator strategy")
	// ErrInvalidRequest indicates that the scaling planner request is invalid.
	ErrInvalidRequest = errors.New("invalid planner request")
	// ErrServiceInitFailed indicates that the ScalingPlannerService cannot initialize.
	ErrServiceInitFailed = fmt.Errorf(commonerrors.FmtInitFailed, ServiceName)
	// ErrStartFailed indicates that the  ScalingPlannerService cannot start.
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
