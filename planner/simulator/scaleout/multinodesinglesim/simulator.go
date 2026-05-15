// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package multinode provides implementation and helper routines of a ScaleOutSimulator that performs simulations that scale
// multiple nodes for a single scale-out simulation
package multinodesinglesim

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/planner/simulator/scaleout"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
)

var (
	_ plannerapi.ScaleOutSimulator = (*defaultSimulator)(nil)
)

type defaultSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	storageMetaAccess plannerapi.StorageMetaAccess
	state             *scaleout.SimulatorState
	traceDir          string
	simulatorConfig   plannerapi.SimulatorConfig
}

// New creates a new [plannerapi.ScaleOutSimulator] that runs simulations sequentially scaling multiple nodes from
// different NodeTemplates at the same priority.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleOutSimulator, error) {
	return &defaultSimulator{
		simulatorConfig:   args.Config,
		viewAccess:        args.ViewAccess,
		schedulerLauncher: args.SchedulerLauncher,
		storageMetaAccess: args.StorageMetaAccess,
		traceDir:          args.TraceDir,
	}, nil
}

func (s *defaultSimulator) Close() error {
	return s.state.Reset()
}

func (s *defaultSimulator) Simulate(ctx context.Context, request *plannerapi.Request, simulationFactory plannerapi.SimulationFactory) (planResult <-chan plannerapi.ScaleOutPlanResult) {
	s.state = scaleout.NewSimulatorState(request, s.simulatorConfig, simulationFactory, s.viewAccess)
	go func() {
		defer close(s.state.ResultCh)
		if err := s.doSimulate(ctx); err != nil {
			scaleout.SendPlanError(s.state.ResultCh, request.GetRef(), err)
		}
	}()
	return s.state.ResultCh
}

func (s *defaultSimulator) doSimulate(ctx context.Context) (err error) {
	if err = s.state.InitializeView(ctx); err != nil {
		return
	}
	s.state.SimulationGroups, err = s.createAndGroupSimulations(ctx)
	if err != nil {
		return
	}
	err = s.runAllGroups(ctx)
	return
}

func (s *defaultSimulator) createAndGroupSimulations(ctx context.Context) ([]plannerapi.ScaleOutSimGroup, error) {
	var (
		allScaleOutNodeTemplates = scaleout.CreateAllNodeTemplates(s.state.Request.Constraint.Spec.NodePools)
		templatesByPriority      = scaleout.GroupScaleOutNodeTemplatesByPriority(allScaleOutNodeTemplates)
		allSimulations           = make([]plannerapi.ScaleOutSimulation, 0, len(templatesByPriority))
		log                      = logr.FromContextOrDiscard(ctx)
		simNum                   = 0
	)
	for pk, templates := range templatesByPriority {
		simulationName := fmt.Sprintf("sim-%d_%s", simNum, pk.String())
		simArgs := plannerapi.ScaleOutSimArgs{
			Name:              simulationName,
			RunCounter:        s.state.SimRunCounter,
			SchedulerLauncher: s.schedulerLauncher,
			StorageMetaAccess: s.storageMetaAccess,
			Config:            s.simulatorConfig,
			NodeTemplates:     templates,
			Strategy:          commontypes.SimulatorStrategyMultiNodeSingleSim,
		}
		sim, err := s.state.SimulationFactory.NewScaleOut(simArgs)
		if err != nil {
			return nil, err
		}
		log.V(3).Info("created simulation", "simulationName", simulationName)
		allSimulations = append(allSimulations, sim)
		simNum++
	}
	return scaleout.CreateScaleOutSimGroups(s.state.Request.GetRef(), allSimulations)
}

func (s *defaultSimulator) runAllGroups(ctx context.Context) (err error) {
	var (
		log           = logr.FromContextOrDiscard(ctx)
		groupPassView = s.state.RequestView()
		simResults    []plannerapi.ScaleOutSimResult
		allSimResults []plannerapi.ScaleOutSimResult
	)
	for groupIndex, group := range s.state.SimulationGroups {
		log := log.WithValues("groupIndex", groupIndex, "groupName", group.Name()) // in-loop log enhanced with further params
		passCtx := logr.NewContext(ctx, log)
		if simResults, groupPassView, err = s.runPassForGroup(passCtx, group, groupPassView); err != nil {
			return
		}
		if len(simResults) > 0 {
			allSimResults = append(allSimResults, simResults...)
		}
	}
	if s.state.Request.AdviceGenerationMode.IsAllAtOnce() {
		err = sendPlanResultUsingSimResults(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(), allSimResults)
	}
	return
}

func (s *defaultSimulator) runPassForGroup(ctx context.Context, group plannerapi.ScaleOutSimGroup, groupPassView minkapi.View) (simResults []plannerapi.ScaleOutSimResult, nextGroupPassView minkapi.View, err error) {
	simResults, err = group.Run(ctx, func(ctx context.Context, name string) (minkapi.View, error) {
		return s.state.CreateSandboxView(ctx, name, groupPassView)
	})
	if err != nil {
		return
	}
	if len(simResults) == 0 {
		nextGroupPassView = groupPassView
		return
	}
	nextGroupPassView = simResults[0].View // all simResults share the same View in this strategy
	if s.state.Request.AdviceGenerationMode.IsIncremental() {
		err = sendPlanResultUsingSimResults(ctx, s.state.ResultCh, s.state.Request, s.state.SimRunCounter.Load(), simResults)
	}
	return
}

// sendPlanResultUsingSimResults constraints a [plannerapi.ScaleOutPlanResult] from the given slice of
// [plannerapi.ScaleOutSimResult] and referring the given [plannerapi.Request] and sends the same on the given result
// channel.
func sendPlanResultUsingSimResults(ctx context.Context,
	resultCh chan<- plannerapi.ScaleOutPlanResult,
	req *plannerapi.Request, simulationRunCount uint32, // TODO: introduce a plannerapi.Metrics.
	simResults []plannerapi.ScaleOutSimResult) error {
	log := logr.FromContextOrDiscard(ctx)
	labels := scaleout.CreatePlanLabels(req, simulationRunCount)
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	var scaleOutPlan sacorev1alpha1.ScaleOutPlan
	for _, sr := range simResults {
		for _, item := range sr.Items {
			existingCount := existingNodeCountByPlacement[item.NodePlacement]
			item.CurrentReplicas = existingCount
			scaleOutPlan.Items = append(scaleOutPlan.Items, item)
		}
		scaleOutPlan.UnsatisfiedPodNames = objutil.GetFullNames(sr.LeftoverUnscheduledPods)
	}
	planResult := plannerapi.ScaleOutPlanResult{
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	log.V(2).Info("Sent Planner Success Response", "response", planResult)
	resultCh <- planResult
	return nil
}
