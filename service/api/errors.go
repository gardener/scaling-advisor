package api

import (
	"errors"
	"fmt"
)

var (
	// ErrInitFailed is a sentinel error indicating that the scaling-advisor service failed to initialize.
	ErrInitFailed = fmt.Errorf("failed to initialize %s service", ServiceName)
	// ErrGenScalingAdvice is a sentinel error indicating that the service failed to generate scaling advice.
	ErrGenScalingAdvice = errors.New("failed to generate scaling advice")
	// ErrLoadSchedulerConfig is a sentinel error indicating that the service failed to load the scheduler configuration.
	ErrLoadSchedulerConfig = errors.New("failed to load scheduler configuration")
	// ErrLaunchScheduler is a sentinel error indicating that the service failed to launch the scheduler.
	ErrLaunchScheduler = errors.New("failed to launch scheduler")
)
