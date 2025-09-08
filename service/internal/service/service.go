// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	mkcore "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/gardener/scaling-advisor/service/internal/service/generator"
	"github.com/gardener/scaling-advisor/service/internal/service/simulation"
	"github.com/go-logr/logr"
)

var _ svcapi.ScalingAdvisorService = (*defaultScalingAdvisor)(nil)

type defaultScalingAdvisor struct {
	minKAPIServer     mkapi.Server
	schedulerLauncher svcapi.SchedulerLauncher
	generator         *generator.Generator
}

func New(log logr.Logger,
	config svcapi.ScalingAdvisorServiceConfig,
	pricer svcapi.InstanceTypeInfoAccess,
	weightsFn svcapi.GetWeightsFunc,
	scorer svcapi.NodeScorer,
	selector svcapi.NodeScoreSelector) (svc svcapi.ScalingAdvisorService, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrInitFailed, err)
		}
	}()
	schedulerLauncher, err := scheduler.NewLauncher(config.SchedulerConfigPath, config.MaxConcurrentSimulations)
	if err != nil {
		return
	}
	minKAPIServer, err := mkcore.NewDefaultInMemory(log, config.MinKAPIConfig)
	if err != nil {
		return
	}
	g, err := generator.New(&generator.Args{
		Pricer:                   pricer,
		WeightsFn:                weightsFn,
		Scorer:                   scorer,
		Selector:                 selector,
		CreateSimFn:              simulation.New,
		CreateSimGroupsFn:        simulation.CreateSimulationGroups,
		MaxConcurrentSimulations: 1, // TODO: This should be a top-level config for the scaling advisor
	})
	if err != nil {
		return
	}
	svc = &defaultScalingAdvisor{
		minKAPIServer:     minKAPIServer,
		schedulerLauncher: schedulerLauncher,
		generator:         g,
	}
	return
}

func (d *defaultScalingAdvisor) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrInitFailed, err)
		}
	}()
	if err = d.minKAPIServer.Start(ctx); err != nil {
		return
	}
	return
}

func synchronizeBaseView(view mkapi.View, cs *svcapi.ClusterSnapshot) error {
	// TODO implement delta cluster snapshot to update the base view before every simulation run which will synchronize
	// the base view with the current state of the target cluster.
	view.Reset()
	for _, nodeInfo := range cs.Nodes {
		if err := view.CreateObject(typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo)); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if err := view.CreateObject(typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod)); err != nil {
			return err
		}
	}
	for _, pc := range cs.PriorityClasses {
		if err := view.CreateObject(typeinfo.PriorityClassesDescriptor.GVK, &pc); err != nil {
			return err
		}
	}
	// TODO: also populate RuntimeClasses after support for the same is introduced in minkapi
	return nil
}

func (d *defaultScalingAdvisor) Stop(ctx context.Context) error {
	if d.minKAPIServer != nil {
		return d.minKAPIServer.Stop(ctx)
	}
	return nil
}

func (d *defaultScalingAdvisor) GenerateAdvice(ctx context.Context, request svcapi.ScalingAdviceRequest) <-chan svcapi.ScalingAdviceEvent {
	adviceEventCh := make(chan svcapi.ScalingAdviceEvent)
	go func() {
		unscheduledPods := podutil.GetPodResourceInfos(request.Snapshot.GetUnscheduledPods())
		if len(unscheduledPods) == 0 {
			generator.SendError(adviceEventCh, request.ScalingAdviceRequestRef, fmt.Errorf("%w: no unscheduled pods found", svcapi.ErrNoUnscheduledPods))
			return
		}
		baseView := d.minKAPIServer.GetBaseView()
		err := synchronizeBaseView(baseView, request.Snapshot)
		if err != nil {
			generator.SendError(adviceEventCh, request.ScalingAdviceRequestRef, err)
			return
		}
		genCtx := logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", request.ID, "correlationID", request.CorrelationID))
		runArgs := &generator.RunArgs{
			BaseView: d.minKAPIServer.GetBaseView(),
			SandboxViewFn: func(log logr.Logger, name string) (mkapi.View, error) {
				return d.minKAPIServer.GetSandboxView(log, name)
			},
			Request:       request, //TODO: backoff component should adjust the return depending on feedback before passing here
			AdviceEventCh: adviceEventCh,
		}
		d.generator.Run(genCtx, runArgs)
	}()
	return adviceEventCh
}
