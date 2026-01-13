// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/go-logr/logr"
	"golang.org/x/sync/semaphore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/events"
	logsapiv1 "k8s.io/component-base/logs/api/v1"
	"k8s.io/kubernetes/pkg/scheduler"
	schedulerapiconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedulerapiconfigv1 "k8s.io/kubernetes/pkg/scheduler/apis/config/v1"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

var _ planner.SchedulerLauncher = (*schedulerLauncher)(nil)

type schedulerLauncher struct {
	schedulerConfig *schedulerapiconfig.KubeSchedulerConfiguration
	semaphore       *semaphore.Weighted
}

var _ planner.SchedulerHandle = (*schedulerHandle)(nil)

type schedulerHandle struct {
	ctx       context.Context
	scheduler *scheduler.Scheduler
	cancelFn  context.CancelFunc
	params    *planner.SchedulerLaunchParams
	name      string
}

// NewLauncher initializes and returns a SchedulerLauncher using a scheduler config file and a maximum parallelism limit.
// It reads the scheduler configuration from the provided file path and validates it.
// Returns an error if the configuration file cannot be read or parsed.
// Then delegates to NewLauncherFromConfig
func NewLauncher(schedulerConfigPath string, maxParallel int) (planner.SchedulerLauncher, error) {
	// Initialize the scheduler with the provided configuration
	configBytes, err := os.ReadFile(filepath.Clean(schedulerConfigPath))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", planner.ErrLoadSchedulerConfig, err)
	}
	return NewLauncherFromConfig(configBytes, maxParallel)
}

// NewLauncherFromConfig initializes and returns a SchedulerLauncher using the scheduler config bytes (YAML) and a maximum parallelism limit.
// It parses the scheduler configuration and validates it. Returns an error if the configuration file cannot be
// parsed.
// maxParallel represents the maximum number of parallel embedded scheduler instances that are launchable via SchedulerLauncher.Launch.
// Once crossed, further calls to SchedulerLauncher.Launch will block until previously obtained SchedulerHandle's are stopped.
func NewLauncherFromConfig(configBytes []byte, maxParallel int) (planner.SchedulerLauncher, error) {
	scheduledConfig, err := parseSchedulerConfig(configBytes)
	if err != nil {
		return nil, err
	}
	return &schedulerLauncher{
		schedulerConfig: scheduledConfig,
		semaphore:       semaphore.NewWeighted(int64(maxParallel)),
	}, nil
}

func parseSchedulerConfig(configBytes []byte) (config *schedulerapiconfig.KubeSchedulerConfiguration, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", planner.ErrParseSchedulerConfig, err)
		}
	}()

	scheme := runtime.NewScheme()
	if err = schedulerapiconfig.AddToScheme(scheme); err != nil {
		return
	}
	if err = schedulerapiconfigv1.AddToScheme(scheme); err != nil {
		return
	}
	codecs := serializer.NewCodecFactory(scheme)
	obj, _, err := codecs.UniversalDecoder(schedulerapiconfig.SchemeGroupVersion).Decode(configBytes, nil, nil)
	if err != nil {
		return
	}
	config = obj.(*schedulerapiconfig.KubeSchedulerConfiguration)
	return config, nil
}

func (s *schedulerLauncher) Launch(ctx context.Context, params *planner.SchedulerLaunchParams) (planner.SchedulerHandle, error) {
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
		log.Info("Begin run scheduler", "name", handle.name)
		handle.scheduler.Run(schedulerCtx)
		log.Info("End run scheduler", "name", handle.name)
	}()
	return handle, nil
}

func (s *schedulerLauncher) createSchedulerHandle(ctx context.Context, cancelFn context.CancelFunc, params *planner.SchedulerLaunchParams) (handle *schedulerHandle, err error) {
	defer func() {
		if err != nil {
			cancelFn()
			err = fmt.Errorf("%w: %w", planner.ErrLaunchScheduler, err)
		}
	}()

	verbosity := logutil.VerbosityFromContext(ctx)
	if verbosity > 0 {
		loggingConfig := logsapiv1.LoggingConfiguration{
			Format:         logsapiv1.DefaultLogFormat,
			FlushFrequency: logsapiv1.TimeOrMetaDuration{Duration: metav1.Duration{Duration: time.Second * 1}},
			Verbosity:      logsapiv1.VerbosityLevel(verbosity),
			Options:        logsapiv1.FormatOptions{},
		}
		err = logsapiv1.ValidateAndApply(&loggingConfig, nil)
		if err != nil {
			err = fmt.Errorf("failed to apply logging configuration: %w", err)
			return
		}
	}

	log := logr.FromContextOrDiscard(ctx)
	broadcaster := events.NewBroadcaster(params.EventSink)
	broadcaster.StartRecordingToSink(ctx.Done())
	name := "embedded-scheduler-" + rand.String(5)
	recorderFactory := profile.NewRecorderFactory(broadcaster)

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

func (s *schedulerHandle) Close() error {
	log := logr.FromContextOrDiscard(s.ctx)
	log.Info("Stopping scheduler", "name", s.name)
	s.cancelFn()
	log.Info("Stopped scheduler", "name", s.name)
	return nil
}

func (s *schedulerHandle) GetParams() planner.SchedulerLaunchParams {
	return *s.params
}
