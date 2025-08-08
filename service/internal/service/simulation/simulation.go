package simulation

import (
	"context"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/core/typeinfo"
	"github.com/gardener/scaling-advisor/service/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

type Simulation struct {
	args            *Args
	schedulerHandle api.SchedulerHandle
	simNode         *corev1.Node
	unscheduledPods []cache.ObjectName
	scheduledPods   map[string][]cache.ObjectName
}

type Args struct {
	Name              string
	Region            string
	AvailabilityZone  string
	Labels            map[string]string
	Annotations       map[string]string
	Taints            []corev1.Taint
	NodeTemplate      sacorev1alpha1.NodeTemplate
	SchedulerLauncher api.SchedulerLauncher
	View              mkapi.View
	ResultCh          chan struct{}
}

func New(args *Args) *Simulation {
	return &Simulation{
		args: args,
	}
}

func (s *Simulation) Start(ctx context.Context) error {
	s.simNode = s.buildSimulationNode()
	if err := s.args.View.CreateObject(typeinfo.NodesDescriptor.GVK, s.simNode); err != nil {
		return err
	}
	schedulerHandle, err := s.launchSchedulerForSimulation(ctx, s.args.View)
	if err != nil {
		return err
	}
	s.schedulerHandle = schedulerHandle
	go func() {
		s.track()
	}()

	select {
	case <-ctx.Done():
		s.schedulerHandle.Stop()
		return ctx.Err()
	case <-s.args.ResultCh:
		// Simulation completed, stop the scheduler and clean up.
		s.schedulerHandle.Stop()
		return nil
	}
}

func (s *Simulation) launchSchedulerForSimulation(ctx context.Context, simView mkapi.View) (api.SchedulerHandle, error) {
	client, dynClient := simView.GetClients()
	informerFactory, dynInformerFactory := simView.GetInformerFactories()
	schedLaunchParams := &api.SchedulerLaunchParams{
		Client:             client,
		DynClient:          dynClient,
		InformerFactory:    informerFactory,
		DynInformerFactory: dynInformerFactory,
		EventSink:          simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *Simulation) buildSimulationNode() *corev1.Node {
	/*
		create a simulation node based on the provided template, region, zone, labels, and taints.
		Add apiconstants.LabelSimulationID with the value of simulationName to the labels.
	*/
	return &corev1.Node{}
}

// track monitors the EventSink for scheduling events for all the unscheduled pods for a simulation run.
func (s *Simulation) track() {
	/*
			Get all the unscheduled pods from the simulation view.
			This function starts a loop which does the following till one of the following conditions is met:
		      1. All the pods are scheduled within the stabilization period OR
		      2. Stabilization period is over and there are still unscheduled pods.

			At the end of the loop it does the following:
			1. Updates the Simulation with unscheduled and scheduled pods.
			2. Sends a signal to the ResultCh channel to indicate that the simulation is done and exits.
	*/
}
