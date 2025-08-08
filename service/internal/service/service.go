package service

import (
	"context"
	"fmt"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	mkcore "github.com/gardener/scaling-advisor/minkapi/core"
	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/gardener/scaling-advisor/service/internal/scheduler"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

var _ api.ScalingAdvisorService = (*defaultScalingAdvisor)(nil)

type defaultScalingAdvisor struct {
	minKAPIConfig     mkapi.MinKAPIConfig
	minKAPIServer     mkapi.Server
	schedulerLauncher api.SchedulerLauncher
}

func New(config api.ScalingAdvisorServiceConfig) (api.ScalingAdvisorService, error) {
	schedulerLauncher, err := scheduler.NewLauncher(config.SchedulerConfigPath, config.MaxConcurrentSimulations)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	return &defaultScalingAdvisor{
		minKAPIConfig:     config.MinKAPIConfig,
		schedulerLauncher: schedulerLauncher,
	}, nil
}

func (d *defaultScalingAdvisor) GenerateScalingAdvice(ctx context.Context, request api.ScalingAdviceRequest, responseFn api.ScalingAdviceResponseFn) (err error) {
	// wraps all errors with api.ErrGenScalingAdvice before return the error if any.
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrGenScalingAdvice, err)
		}
	}()
	genCtx := logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues("requestID", request.ID, "correlationID", request.CorrelationID))
	if err = d.initMinKAPIServer(genCtx); err != nil {
		return
	}
	if err = d.populateBaseView(request.Snapshot); err != nil {
		return
	}

	var (
		counter         int
		simView         mkapi.View
		schedulerHandle api.SchedulerHandle
	)

	for _, nodePool := range request.Constraint.Spec.NodePools {
		nodeTemplatePartitions := partitionByPriority(nodePool.NodeTemplates)
		for _, nodeTemplates := range nodeTemplatePartitions {
			for _, nodeTemplate := range nodeTemplates {
				for _, zone := range nodePool.AvailabilityZones {
					simView, err = d.minKAPIServer.GetSimulationView()
					if err != nil {
						return
					}
					simulationID := fmt.Sprintf("%s-%s-%s-%d", nodePool.Name, zone, nodeTemplate.Name, counter)
					counter++
					simNode := buildSimulationNode(simulationID, nodePool.Region, zone, nodeTemplate, nodePool.Labels, nodePool.Annotations, nodePool.Taints)
					if err = simView.CreateObject(typeinfo.NodesDescriptor.GVK, simNode); err != nil {
						return
					}
					schedulerHandle, err = d.launchSchedulerForSimulation(ctx, simView)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return
}

func (d *defaultScalingAdvisor) launchSchedulerForSimulation(ctx context.Context, simView mkapi.View) (api.SchedulerHandle, error) {
	client, dynClient := simView.GetClients()
	informerFactory, dynInformerFactory := simView.GetInformerFactories()
	schedLaunchParams := &api.SchedulerLaunchParams{
		Client:             client,
		DynClient:          dynClient,
		InformerFactory:    informerFactory,
		DynInformerFactory: dynInformerFactory,
		EventSink:          simView.GetEventSink(),
	}
	return d.schedulerLauncher.Launch(ctx, schedLaunchParams)
}

func partitionByPriority(nodeTemplates []sacorev1alpha1.NodeTemplate) [][]sacorev1alpha1.NodeTemplate {
	// Partition the node templates by priority.
	// TODO implement me
	return nil
}

func (d *defaultScalingAdvisor) initMinKAPIServer(ctx context.Context) error {
	if d.minKAPIServer != nil {
		return nil
	}
	server, err := mkcore.NewInMemoryMinKAPI(ctx, d.minKAPIConfig)
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	if err = server.Start(); err != nil {
		return fmt.Errorf("%w: %w", api.ErrInitFailed, err)
	}
	return nil
}

func (d *defaultScalingAdvisor) populateBaseView(clusterSnapshot api.ClusterSnapshot) error {
	// TODO implement me
	return nil
}
