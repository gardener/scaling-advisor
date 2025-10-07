// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/go-logr/logr"
	"golang.org/x/sync/semaphore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/scheduler"
	schedulerapiconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedulerapiconfigv1 "k8s.io/kubernetes/pkg/scheduler/apis/config/v1"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

var _ svcapi.SchedulerLauncher = (*schedulerLauncher)(nil)

type schedulerLauncher struct {
	schedulerConfig *schedulerapiconfig.KubeSchedulerConfiguration
	semaphore       *semaphore.Weighted
}

var _ svcapi.SchedulerHandle = (*schedulerHandle)(nil)

type schedulerHandle struct {
	ctx       context.Context
	name      string
	scheduler *scheduler.Scheduler
	cancelFn  context.CancelFunc
	params    *svcapi.SchedulerLaunchParams
}

func NewLauncher(schedulerConfigPath string, maxParallel int) (svcapi.SchedulerLauncher, error) {
	// Initialize the scheduler with the provided configuration
	scheduledConfig, err := loadSchedulerConfig(schedulerConfigPath)
	if err != nil {
		return nil, err
	}
	return &schedulerLauncher{
		schedulerConfig: scheduledConfig,
		semaphore:       semaphore.NewWeighted(int64(maxParallel)),
	}, nil
}

func loadSchedulerConfig(configPath string) (config *schedulerapiconfig.KubeSchedulerConfiguration, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", svcapi.ErrLoadSchedulerConfig, err)
		}
	}()

	data, err := os.ReadFile(filepath.Clean(configPath))
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

func (s *schedulerLauncher) Launch(ctx context.Context, params *svcapi.SchedulerLaunchParams) (svcapi.SchedulerHandle, error) {
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
		log.Info("Running scheduler", "name", handle.name)
		handle.scheduler.Run(schedulerCtx)
		log.Info("Stopped scheduler", "name", handle.name)
	}()
	return handle, nil
}

func (s *schedulerLauncher) createSchedulerHandle(ctx context.Context, cancelFn context.CancelFunc, params *svcapi.SchedulerLaunchParams) (handle *schedulerHandle, err error) {
	defer func() {
		if err != nil {
			cancelFn()
			err = fmt.Errorf("%w: %w", svcapi.ErrLaunchScheduler, err)
		}
	}()
	log := logr.FromContextOrDiscard(ctx)
	broadcaster := events.NewBroadcaster(params.EventSink)
	broadcaster.StartRecordingToSink(ctx.Done())
	//
	name := "embedded-scheduler-" + rand.String(5)
	recorderFactory := profile.NewRecorderFactory(broadcaster) //
	// Explicit recorder factory using your instance name
	sched, err := scheduler.New(
		ctx,
		params.Client,
		params.InformerFactory,
		params.DynInformerFactory,
		recorderFactory,
		scheduler.WithProfiles(s.schedulerConfig.Profiles...),
		scheduler.WithPercentageOfNodesToScore(s.schedulerConfig.PercentageOfNodesToScore),
		scheduler.WithPodInitialBackoffSeconds(s.schedulerConfig.PodInitialBackoffSeconds),
		scheduler.WithPodMaxBackoffSeconds(s.schedulerConfig.PodMaxBackoffSeconds),
		scheduler.WithExtenders(s.schedulerConfig.Extenders...))
	if err != nil {
		return
	}
	params.InformerFactory.Start(ctx.Done())
	params.DynInformerFactory.Start(ctx.Done())

	params.InformerFactory.WaitForCacheSync(ctx.Done())
	params.DynInformerFactory.WaitForCacheSync(ctx.Done())

	if err = sched.WaitForHandlersSync(ctx); err != nil {
		return
	}
	handle = &schedulerHandle{
		ctx:       ctx,
		name:      "scheduler-" + rand.String(5),
		scheduler: sched,
		cancelFn:  cancelFn,
		params:    params,
	}
	log.V(3).Info("created scheduler handle", "name", name)
	return
}

func (s *schedulerHandle) Stop() {
	log := logr.FromContextOrDiscard(s.ctx)
	log.Info("Stopping scheduler", "name", s.name)
	s.cancelFn()
}

func (s *schedulerHandle) GetParams() svcapi.SchedulerLaunchParams {
	return *s.params
}
