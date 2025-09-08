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
	minKAPIConfig     mkapi.Config
	minKAPIServer     mkapi.Server
	schedulerLauncher svcapi.SchedulerLauncher
	pricer            svcapi.InstanceTypeInfoAccess
	weighsFn          svcapi.GetWeightsFunc
	scorer            svcapi.NodeScorer
	selector          svcapi.NodeScoreSelector
}

func New(config svcapi.ScalingAdvisorServiceConfig,
	pricer svcapi.InstanceTypeInfoAccess,
	weights svcapi.GetWeightsFunc,
	scorer svcapi.NodeScorer,
	selector svcapi.NodeScoreSelector) (svcapi.ScalingAdvisorService, error) {
	schedulerLauncher, err := scheduler.NewLauncher(config.SchedulerConfigPath, config.MaxConcurrentSimulations)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", svcapi.ErrInitFailed, err)
	}
	return &defaultScalingAdvisor{
		minKAPIConfig:     config.MinKAPIConfig,
		schedulerLauncher: schedulerLauncher,
		pricer:            pricer,
		weighsFn:          weights,
		scorer:            scorer,
		selector:          selector,
	}, nil
}

func (d *defaultScalingAdvisor) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrInitFailed, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	d.minKAPIServer, err = mkcore.NewDefaultInMemory(log, d.minKAPIConfig)
	if err != nil {
		return
	}
	if err = d.minKAPIServer.Start(ctx); err != nil {
		return
	}
	return nil
}

func populateBaseView(view mkapi.View, cs *svcapi.ClusterSnapshot) error {
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
	return nil
}

func (d *defaultScalingAdvisor) Stop(ctx context.Context) error {
	if d.minKAPIServer != nil {
		return d.minKAPIServer.Stop(ctx)
	}
	return nil
}

func (d *defaultScalingAdvisor) GenerateAdvice(ctx context.Context, request svcapi.ScalingAdviceRequest) <-chan svcapi.ScalingAdviceEvent {
	eventCh := make(chan svcapi.ScalingAdviceEvent)
	go func() {
		unscheduledPods := getPodResourceInfos(request.Snapshot.GetUnscheduledPods())
		if len(unscheduledPods) == 0 {
			err := svcapi.AsGenerateError(request.ID, request.CorrelationID, fmt.Errorf("%w: no unscheduled pods found", svcapi.ErrNoUnscheduledPods))
			eventCh <- svcapi.ScalingAdviceEvent{
				Err: err,
			}
			return
		}
	}()
	unscheduledPods := getPodResourceInfos(request.Snapshot.GetUnscheduledPods())
	if len(unscheduledPods) == 0 {
		err = api.ErrNoUnscheduledPods
		return
	}

	genCtx := logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", request.ID, "correlationID", request.CorrelationID))
	g := generator.New(genCtx, &generator.Args{
		Pricer:            d.pricer,
		WeightsFn:         d.weighsFn,
		Scorer:            d.scorer,
		Selector:          d.selector,
		CreateSimFn:       simulation.New,
		CreateSimGroupsFn: simulation.CreateSimulationGroups,
		Request:           request,
		ResponseFn:        responseFn,
	})
	return g.Generate()
}

func getPodResourceInfos(podInfos []svcapi.PodInfo) []svcapi.PodResourceInfo {
	podResourceInfos := make([]svcapi.PodResourceInfo, 0, len(podInfos))
	for _, podInfo := range podInfos {
		podResourceInfos = append(podResourceInfos, svcapi.PodResourceInfo{
			UID:                podInfo.UID,
			NamespacedName:     podInfo.NamespacedName,
			AggregatedRequests: podInfo.AggregatedRequests,
		})
	}
	return podResourceInfos
}
