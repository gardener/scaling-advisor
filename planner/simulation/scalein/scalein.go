package scalein

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/go-logr/logr"
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
	d.state = FreshRunState()
	return nil
}

func (d *defaultSimulation) Name() string {
	return d.args.Name
}

func (d *defaultSimulation) Status() plannerapi.ActivityStatus {
	return d.state.status
}

func (d *defaultSimulation) PriorityKey() commontypes.PriorityKey {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Run(ctx context.Context, view minkapi.View) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q, runNum %d: %w", plannerapi.ErrRunSimulation, d.args.Name, d.runNum(), err)
			d.state.err = err
			d.state.status = plannerapi.ActivityStatusFailure
		}
	}()

	if ctx, err = d.state.Init(ctx, d.args.Name, d.incRunNum(), view); err != nil {
		return
	}

	//TODO: implement RemoveNodeAndUnbindPods
	if err = d.state.RemoveNodeAndUnbindPods(d.args.NodeName); err != nil {
		return
	}

	schedulerHandle, err := d.launchSchedulerForSimulation(ctx, view)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(schedulerHandle)

	err = d.workAndTrackUntilStabilized(ctx, view)
	if err != nil {
		return
	}

	nodePodAssignments, err := d.state.NodePodAssignments()
	if err != nil {
		return
	}

	d.result = plannerapi.ScaleInSimRunResult{
		Name:               d.args.Name,
		View:               view,
		Items:              d.state.GetScaleOutItems(),
		NodePodAssignments: d.state.getScaleOutNodeAssignments(),
		PodEvictionReasons: d.state.GetPodEvictionReasons(),
		//OtherNodePodAssignments: nodePodAssignments,
		//LeftoverUnscheduledPods: d.state.leftoverUnscheduledPodNames.UnsortedList(),
	}
	d.state.status = plannerapi.ActivityStatusSuccess
	log := logr.FromContextOrDiscard(ctx)
	if len(d.result.LeftoverUnscheduledPods) > 0 {
		log.V(3).Info("LeftoverUnscheduledPods after run", "leftoverUnscheduledPodCount", len(s.result.LeftoverUnscheduledPods))
	}
	return
}

func (d *defaultSimulation) runNum() uint32 {
	return s.args.RunCounter.Load()
}

func (d *defaultSimulation) incRunNum() uint32 {
	return d.args.RunCounter.Add(1)
}

func (d *defaultSimulation) Result() (plannerapi.ScaleInPlanResult, error) {
	var err error
	switch d.state.status {
	case plannerapi.ActivityStatusPending:
		err = fmt.Errorf("simulation %q is still pending", d.args.Name)
		return plannerapi.ScaleInPlanResult{}, err
	case plannerapi.ActivityStatusRunning:
		err = fmt.Errorf("simulation %q is still running", d.args.Name)
		return plannerapi.ScaleInPlanResult{}, err
	case plannerapi.ActivityStatusFailure:
		err = d.state.err
		return plannerapi.ScaleInPlanResult{}, err
	}
	result := d.result
	return result, nil
}

func validateSimArgs(args *plannerapi.ScaleInSimArgs) error {
	if args.Name == "" {
		return fmt.Errorf("simulation name must not be empty")
	}
	if args.Config.MaxParallelSimulations <= 0 {
		return fmt.Errorf("max parallel simulations: %d must be positive value for simulation %q", args.Config.MaxParallelSimulations, args.Name)
	}
	if args.Config.TrackPollInterval <= 0 {
		return fmt.Errorf("track poll interval %v must be positive duration for simulation %q", args.Config.TrackPollInterval, args.Name)
	}
	if args.Config.MaxUnchangedTrackAttempts <= 0 {
		return fmt.Errorf("max unchanged track attempts %d must be positive for simulation %q", args.Config.MaxUnchangedTrackAttempts, args.Name)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("scheduler launcher must not be nil for simulation %q", args.Name)
	}
	if args.RunCounter == nil {
		return fmt.Errorf("run counter must not be nil for simulation %q", args.Name)
	}
	if args.RunCounter.Load() == 0 {
		return fmt.Errorf("run counter must have a non-zero initial value for simulation %q", args.Name)
	}
	return nil
}
