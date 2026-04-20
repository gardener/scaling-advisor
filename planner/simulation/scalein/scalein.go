package scalein

import (
	"context"
	"fmt"

	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ plannerapi.ScaleInSimulation = (*defaultSimulation)(nil)

// defaultSimulation is the default implementation of a ScaleInSimulation.
type defaultSimulation struct {
	args   *plannerapi.ScaleInSimArgs
	result plannerapi.ScaleInSimRunResult
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

	unboundPods, err := d.state.RemoveNodeAndUnbindPods(d.args.NodeName)
	if err != nil {
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

	nodePodAssignments, err := d.state.NodePodAssignments(unboundPods)
	if err != nil {
		return
	}

	d.result = plannerapi.ScaleInSimRunResult{
		Name:               d.args.Name,
		View:               view,
		Items:              d.state.GetScaleInItems(),
		NodePodAssignments: nodePodAssignments,
		PodsToReschedule:   d.state.GetPodsToReschedule(),
	}
	d.state.status = plannerapi.ActivityStatusSuccess
	log := logr.FromContextOrDiscard(ctx)
	if len(d.result.PodsToReschedule) > 0 {
		log.V(3).Info("LeftoverUnscheduledPods after scale-in run", "PodsToReschedule", len(d.result.PodsToReschedule))
	}
	return
}

func (d *defaultSimulation) launchSchedulerForSimulation(ctx context.Context, simView minkapi.View) (plannerapi.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &plannerapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return d.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

// workAndTrackUntilStabilized starts a loop which performs work and tracks the state of the simulation until one of the following conditions is met:
//  1. All the pods are scheduled.
//  2. Events have stabilized. i.e., no more scheduling events within maxUnchangedTrackAttempts
//  3. Context timeout.
//  4. Any error
func (d *defaultSimulation) workAndTrackUntilStabilized(ctx context.Context, view minkapi.View) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	var stabilized bool
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			if err = d.doWork(ctx, view); err != nil {
				return
			}
			<-time.After(d.args.Config.TrackPollInterval)
			if stabilized, err = d.state.Track(d.args.Config.MaxUnchangedTrackAttempts); err != nil || stabilized {
				return
			}
			if len(d.state.leftoverUnscheduledPodNames) == 0 {
				log.V(2).Info("ending simulation run since leftoverUnscheduledPodNames is zero", "numTrackAttempts", d.state.numTrackAttempts)
				return
			}
		}
	}
}

// doWork does miscellaneous simulation work to ensure that the kube-scheduler can
// continue pod-node bindings. Currently, it delegates to BindClaimsAndVolumesWithNonNilClaimRefs and if the parent
// SimulatorStrategy supports multiple node scaling, a call is issued to CreateSimulationNodes
func (d *defaultSimulation) doWork(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(3).Info("Invoked doWork", "viewName", view.GetName())
	numBound, err := volutil.FinalizeStaticBindingsForSelectedClaims(ctx, view)
	if err != nil {
		return err
	}
	if numBound > 0 {
		log.V(3).Info("Reset RunState.numUnchangedTrackAttempts since BindClaimsAndVolumesWithNonNilClaimRefs performed work", "numBound", numBound)
		d.state.numUnchangedTrackAttempts = 0
	}
	return err
}

func (d *defaultSimulation) runNum() uint32 {
	return d.args.RunCounter.Load()
}

func (d *defaultSimulation) incRunNum() uint32 {
	return d.args.RunCounter.Add(1)
}

func (d *defaultSimulation) Result() (plannerapi.ScaleInSimRunResult, error) {
	var err error
	switch d.state.status {
	case plannerapi.ActivityStatusPending:
		err = fmt.Errorf("simulation %q is still pending", d.args.Name)
		return plannerapi.ScaleInSimRunResult{}, err
	case plannerapi.ActivityStatusRunning:
		err = fmt.Errorf("simulation %q is still running", d.args.Name)
		return plannerapi.ScaleInSimRunResult{}, err
	case plannerapi.ActivityStatusFailure:
		err = d.state.err
		return plannerapi.ScaleInSimRunResult{}, err
	}
	result := d.result
	return result, nil
}

func (d *defaultSimulation) NextCandidate(ctx context.Context, args plannerapi.ScaleInCandidateArgs, skipNodes *sets.Set[string]) (*corev1.Node, error) {
	return d.args.CandidateSelector.NextCandidate(ctx, args, skipNodes)
}

func validateSimArgs(args *plannerapi.ScaleInSimArgs) error {
	if args.Name == "" {
		return fmt.Errorf("simulation name must not be empty")
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
