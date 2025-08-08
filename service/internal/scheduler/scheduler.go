package scheduler

import (
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/go-logr/logr"
	"golang.org/x/sync/semaphore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/scheduler"
	schedulerapiconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedulerapiconfigv1 "k8s.io/kubernetes/pkg/scheduler/apis/config/v1"
	"k8s.io/kubernetes/pkg/scheduler/profile"
	"os"
)

var _ api.SchedulerLauncher = (*schedulerLauncher)(nil)

type schedulerLauncher struct {
	schedulerConfig *schedulerapiconfig.KubeSchedulerConfiguration
	semaphore       *semaphore.Weighted
}

var _ api.SchedulerHandle = (*schedulerHandle)(nil)

type schedulerHandle struct {
	ctx       context.Context
	name      string
	scheduler *scheduler.Scheduler
	cancelFn  context.CancelFunc
	params    *api.SchedulerLaunchParams
}

func NewLauncher(schedulerConfigPath string, maxConcurrent int) (api.SchedulerLauncher, error) {
	// Initialize the scheduler with the provided configuration
	scheduledConfig, err := loadSchedulerConfig(schedulerConfigPath)
	if err != nil {
		return nil, err
	}
	return &schedulerLauncher{
		schedulerConfig: scheduledConfig,
		semaphore:       semaphore.NewWeighted(int64(maxConcurrent)),
	}, nil
}

func loadSchedulerConfig(configPath string) (config *schedulerapiconfig.KubeSchedulerConfiguration, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", api.ErrLoadSchedulerConfig, err)
		}
	}()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	scheme := runtime.NewScheme()
	if err = schedulerapiconfig.AddToScheme(scheme); err != nil {
		return
	}
	if err = schedulerapiconfigv1.AddToScheme(scheme); err != nil {
		return
	}
	codecs := serializer.NewCodecFactory(scheme)
	obj, _, err := codecs.UniversalDecoder(schedulerapiconfig.SchemeGroupVersion).Decode(data, nil, nil)
	if err != nil {
		return
	}
	config = obj.(*schedulerapiconfig.KubeSchedulerConfiguration)
	return config, nil
}

func (s *schedulerLauncher) Launch(ctx context.Context, params *api.SchedulerLaunchParams) (api.SchedulerHandle, error) {
	log := logr.FromContextOrDiscard(ctx)
	if err := s.semaphore.Acquire(ctx, 1); err != nil {
		return nil, err
	}

	schedulerCtx, cancelFn := context.WithCancel(ctx)
	handle, err := s.createSchedulerHandle(schedulerCtx, cancelFn, params)
	if err != nil {
		return nil, err
	}

	go func() {
		log.Info("Launching scheduler", "name", handle.name)
		handle.scheduler.Run(schedulerCtx)
	}()
	return handle, nil
}

func (s *schedulerLauncher) createSchedulerHandle(ctx context.Context, cancelFn context.CancelFunc, params *api.SchedulerLaunchParams) (handle *schedulerHandle, err error) {
	defer func() {
		if err != nil {
			cancelFn()
			err = fmt.Errorf("%w: %w", api.ErrLaunchScheduler, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	informerFactory := informers.NewSharedInformerFactory(params.Client, 0)
	dynInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(params.DynClient, 0)
	broadcaster := events.NewBroadcaster(params.EventSink)
	recorderFactory := profile.NewRecorderFactory(broadcaster)
	sched, err := scheduler.New(
		ctx,
		params.Client,
		informerFactory,
		dynInformerFactory,
		recorderFactory,
		scheduler.WithProfiles(s.schedulerConfig.Profiles...),
		scheduler.WithPercentageOfNodesToScore(s.schedulerConfig.PercentageOfNodesToScore),
		scheduler.WithPodInitialBackoffSeconds(s.schedulerConfig.PodInitialBackoffSeconds),
		scheduler.WithPodMaxBackoffSeconds(s.schedulerConfig.PodMaxBackoffSeconds),
		scheduler.WithExtenders(s.schedulerConfig.Extenders...))
	if err != nil {
		return
	}
	informerFactory.Start(ctx.Done())
	dynInformerFactory.Start(ctx.Done())

	informerFactory.WaitForCacheSync(ctx.Done())
	dynInformerFactory.WaitForCacheSync(ctx.Done())

	if err = sched.WaitForHandlersSync(ctx); err != nil {
		return
	}
	log.V(3).Info("scheduler handlers synced")
	log.Info("Starting scheduler.Run with config", "config", s.schedulerConfig)

	handle = &schedulerHandle{
		ctx:       ctx,
		name:      "scheduler-" + rand.String(5),
		scheduler: sched,
		cancelFn:  cancelFn,
		params:    params,
	}
	return
}

func (s *schedulerHandle) Stop() {
	log := logr.FromContextOrDiscard(s.ctx)
	log.Info("Stopping scheduler", "name", s.name)
	s.cancelFn()
}

func (s *schedulerHandle) GetParams() api.SchedulerLaunchParams {
	return *s.params
}
