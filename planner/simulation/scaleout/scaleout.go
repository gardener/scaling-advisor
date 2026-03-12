// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package scaleout provides implementation types for the [plannerapi.ScaleOutSimulation]
package scaleout

import (
	"context"
	"fmt"
	"strings"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/viewutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/go-logr/logr"
)

var _ plannerapi.ScaleOutSimulation = (*defaultSimulation)(nil)

// defaultSimulation is the default implementation of a ScaleOutSimulation.
type defaultSimulation struct {
	args   *plannerapi.ScaleOutSimArgs
	result plannerapi.ScaleOutSimResult
	state  RunState
}

// NewDefault creates a new ScaleOutSimulation instance with the specified name and using the given arguments after validation.
func NewDefault(args plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	if err := validateSimArgs(&args); err != nil {
		return nil, fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	sim := &defaultSimulation{
		args:  &args,
		state: MakeRunState(),
	}
	return sim, nil
}

func (s *defaultSimulation) Reset() error {
	s.state = MakeRunState()
	return nil
}

func (s *defaultSimulation) PriorityKey() commontypes.PriorityKey {
	return s.args.NodeTemplates[0].PriorityKey
}

func (s *defaultSimulation) Name() string {
	return s.args.Name
}

func (s *defaultSimulation) Status() plannerapi.ActivityStatus {
	return s.state.status
}

func (s *defaultSimulation) Result() (result plannerapi.ScaleOutSimResult, err error) {
	switch s.state.status {
	case plannerapi.ActivityStatusPending:
		err = fmt.Errorf("simulation %q is still pending", s.args.Name)
		return
	case plannerapi.ActivityStatusRunning:
		err = fmt.Errorf("simulation %q is still running", s.args.Name)
		return
	case plannerapi.ActivityStatusFailure:
		err = s.state.err
		return
	}
	result = s.result
	return
}

func (s *defaultSimulation) Run(ctx context.Context, view minkapi.View) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q, runNum %d: %w", plannerapi.ErrRunSimulation, s.args.Name, s.runNum(), err)
			s.state.err = err
			s.state.status = plannerapi.ActivityStatusFailure
		}
	}()

	if ctx, err = s.state.Init(ctx, s.args.Name, s.incRunNum(), view, s.args.TraceDir); err != nil {
		return
	}

	if err = s.state.CreateSimulationNodes(s.args.StorageMetaAccess, s.args.NodeTemplates); err != nil {
		return
	}

	schedulerHandle, err := s.launchSchedulerForSimulation(ctx, view)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(schedulerHandle)

	err = s.workAndTrackUntilStabilized(ctx, view)
	if err != nil {
		return
	}

	otherNodePodAssignments, err := s.state.getOtherPodNodeAssignments()
	if err != nil {
		return
	}

	s.result = plannerapi.ScaleOutSimResult{
		Name:                    s.args.Name,
		View:                    view,
		Items:                   s.state.GetScaleOutItems(),
		NodePodAssignments:      s.state.getScaleOutNodeAssignments(),
		OtherNodePodAssignments: otherNodePodAssignments,
		LeftoverUnscheduledPods: s.state.leftoverUnscheduledPodNames.UnsortedList(),
	}
	s.state.status = plannerapi.ActivityStatusSuccess
	log := logr.FromContextOrDiscard(ctx)
	if len(s.result.LeftoverUnscheduledPods) > 0 {
		log.V(3).Info("LeftoverUnscheduledPods after run", "leftoverUnscheduledPodCount", len(s.result.LeftoverUnscheduledPods))
	}
	return
}

// workAndTrackUntilStabilized starts a loop which performs work and tracks the state of the simulation until one of the following conditions is met:
//  1. All the pods are scheduled.
//  2. Events have stabilized. i.e., no more scheduling events within maxUnchangedTrackAttempts
//  3. Context timeout.
//  4. Any error
func (s *defaultSimulation) workAndTrackUntilStabilized(ctx context.Context, view minkapi.View) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	var stabilized bool
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			if err = s.doWork(ctx, view); err != nil {
				return
			}
			<-time.After(s.args.Config.TrackPollInterval)
			if stabilized, err = s.state.Track(s.args.Config.MaxUnchangedTrackAttempts); err != nil || stabilized {
				return
			}
			if len(s.state.leftoverUnscheduledPodNames) == 0 {
				log.V(2).Info("ending simulation run since leftoverUnscheduledPodNames is zero", "numTrackAttempts", s.state.numTrackAttempts)
				return
			}
		}
	}
}

// doWork does miscellaneous simulation work to ensure that the kube-scheduler can
// continue pod-node bindings. Currently, it delegates to BindClaimsAndVolumesWithNonNilClaimRefs and if the parent
// SimulatorStrategy supports multiple node scaling, a call is issued to CreateSimulationNodes
func (s *defaultSimulation) doWork(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(3).Info("Invoked doWork", "viewName", view.GetName())
	provisionedPvs, err := volutil.ProvisionAndBindVolumesFoSelectedClaimsInWFFC(ctx, view)
	if err != nil {
		return err
	}
	if len(provisionedPvs) > 0 {
		log.V(3).Info("ProvisionAndBindVolumesFoSelectedClaimsInWFFC performed work - reset RunState.numUnchangedTrackAttempts since ",
			"numProvisionedPvs", len(provisionedPvs))
		s.state.numUnchangedTrackAttempts = 0
	}
	numBound, err := volutil.FinalizeStaticBindingsForSelectedClaimsInWFFC(ctx, view)
	if err != nil {
		return err
	}
	if numBound > 0 {
		log.V(3).Info("FinalizeStaticBindingsForSelectedClaimsInWFFC performed work - reset RunState.numUnchangedTrackAttempts since ", "numBound", numBound)
		s.state.numUnchangedTrackAttempts = 0
	}
	if s.args.Strategy.IsMultiNode() {
		err = s.state.CreateSimulationNodes(s.args.StorageMetaAccess, s.args.NodeTemplates)
	}
	_ = viewutil.LogObjects(ctx, "doWork done", view)
	return err
}

func validateSimArgs(args *plannerapi.ScaleOutSimArgs) error {
	if len(args.NodeTemplates) == 0 {
		return fmt.Errorf("no ScaleOutNodeTemplate specified for simulation %q", args.Name)
	}
	if args.Config.TrackPollInterval <= 0 {
		return fmt.Errorf("track poll interval must be positive duration for simulation %q", args.Name)
	}
	if args.Config.MaxUnchangedTrackAttempts <= 0 {
		return fmt.Errorf("max unchanged track attempts must be positive for simulation %q", args.Name)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("scheduler launcher must not be nil for simulation %q", args.Name)
	}
	pk := args.NodeTemplates[0].PriorityKey
	for _, t := range args.NodeTemplates {
		if strings.TrimSpace(t.Region) == "" {
			return fmt.Errorf("region unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if strings.TrimSpace(t.AvailabilityZone) == "" {
			return fmt.Errorf("az unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if strings.TrimSpace(t.InstanceType) == "" {
			return fmt.Errorf("instanceType unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if strings.TrimSpace(t.TemplateName) == "" {
			return fmt.Errorf("templateName unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if strings.TrimSpace(t.PoolName) == "" {
			return fmt.Errorf("poolName unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if strings.TrimSpace(t.Architecture) == "" {
			return fmt.Errorf("architecture unset for scale-out node template %q for simulation %q", t.TemplateName, args.Name)
		}
		if t.PriorityKey != pk {
			return fmt.Errorf("priorityKey of all scale-out node templates must be same for simulation %q", args.Name)
		}
	}
	return nil
}

func (s *defaultSimulation) launchSchedulerForSimulation(ctx context.Context, simView minkapi.View) (plannerapi.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &plannerapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *defaultSimulation) runNum() uint32 {
	return s.args.RunCounter.Load()
}

func (s *defaultSimulation) incRunNum() uint32 {
	return s.args.RunCounter.Add(1)
}

