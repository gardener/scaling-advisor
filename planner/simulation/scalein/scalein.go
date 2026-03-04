package scalein

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
)

var _ plannerapi.ScaleInSimulation = (*defaultSimulation)(nil)

// defaultSimulation is the default implementation of a ScaleInSimulation.
type defaultSimulation struct {
	args   *plannerapi.ScaleInSimArgs
	result plannerapi.ScaleInPlanResult
	state  RunState
}

// NewDefault creates a new ScaleInSimulation instance with the specified name and using the given arguments after validation.
func NewDefault(args plannerapi.ScaleInSimArgs) (plannerapi.ScaleInSimulation, error) {
	if err := validateSimArgs(&args); err != nil {
		return nil, fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	sim := &defaultSimulation{
		args:  &args,
		state: FreshRunState(),
	}
	return sim, nil
}

func (d *defaultSimulation) Reset() error {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Name() string {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Status() plannerapi.ActivityStatus {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) PriorityKey() commontypes.PriorityKey {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Run(ctx context.Context, view minkapi.View) error {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Result() (plannerapi.ScaleInSimRunResult, error) {
	//TODO implement me
	panic("implement me")
}

func validateSimArgs(args *plannerapi.ScaleInSimArgs) error {
	//TODO: complete validations for all simulation args
	if args.Config.TrackPollInterval <= 0 {
		return fmt.Errorf("track poll interval must be positive duration for simulation %q", args.Name)
	}
	if args.Config.MaxUnchangedTrackAttempts <= 0 {
		return fmt.Errorf("max unchanged track attempts must be positive for simulation %q", args.Name)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("scheduler launcher must not be nil for simulation %q", args.Name)
	}
	return nil
}
