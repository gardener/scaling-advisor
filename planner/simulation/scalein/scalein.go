package scalein

import (
	"context"
	"fmt"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

func (d *defaultSimulation) Priority() commontypes.Priority {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulation) Run(ctx context.Context, view minkapi.View, node *corev1.Node) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q, runNum %d: %w", plannerapi.ErrRunSimulation, d.args.Name, d.runNum(), err)
			d.state.err = err
			d.state.status = plannerapi.ActivityStatusFailure
		}
	}()

	if ctx, err = d.state.Init(ctx, d.args.Name, d.incRunNum(), view, d.args.TraceDir); err != nil {
		return
	}

	err = d.state.RemoveNodeAndUnbindPods(node.Name)
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

	d.result = plannerapi.ScaleInSimRunResult{
		Name: d.args.Name,
		View: view,
		Item: sacorev1alpha1.ScaleInItem{
			NodePlacement: sacorev1alpha1.NodePlacement{
				PoolName:         node.Labels[commonconstants.LabelNodePoolName],
				TemplateName:     node.Labels[commonconstants.LabelNodeTemplateName],
				InstanceType:     node.Labels[corev1.LabelInstanceTypeStable],
				Region:           node.Labels[corev1.LabelTopologyRegion],
				AvailabilityZone: node.Labels[corev1.LabelTopologyZone],
			},
			NodeName: node.Name,
		},
		IsSimulationSuccess: d.state.IsSimulationSuccess(),
	}
	d.state.status = plannerapi.ActivityStatusSuccess
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

// workAndTrackUntilStabilized drives the per-tick simulation loop. Each tick calls doWork to
// advance side-effecting state (e.g. simulated PV provisioning), waits TrackPollInterval, then
// invokes [RunState.Track] to consume any kube-scheduler events. The loop returns when:
//
//  1. ctx is cancelled or times out — returns ctx.Err().
//  2. doWork or Track returns an error — returns that error.
//  3. Track reports stabilized (no events for MaxUnchangedTrackAttempts consecutive polls).
//  4. pendingPods is empty (every displaced pod was rescheduled or moved to currentUnscheduledPods).
//
// On normal termination (cases 3 and 4) the returned error is nil; the caller inspects
// [RunState.IsSimulationSuccess] to decide whether the run actually succeeded.
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
			if len(d.state.pendingPods) == 0 {
				log.V(2).Info("ending simulation run since pendingPods is zero", "numTrackAttempts", d.state.numTrackAttempts)
				return
			}
		}
	}
}

// doWork performs the per-tick side-effecting work the kube-scheduler cannot do on its own:
// provisioning a simulated PV for any WFFC PVC the scheduler has annotated with a selected
// node, and finalizing static-binding metadata for WFFC PVCs whose PV the scheduler has chosen.
// When either step actually does work, numUnchangedTrackAttempts is reset so the stabilization
// counter reflects the progress.
func (d *defaultSimulation) doWork(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(3).Info("Invoked doWork", "viewName", view.GetName())
	provisionedPVs, err := volutil.ProvisionAndBindVolumesFoSelectedClaimsInWFFC(ctx, view)
	if err != nil {
		return err
	}
	if len(provisionedPVs) > 0 {
		log.V(3).Info("ProvisionAndBindVolumesFoSelectedClaimsInWFFC performed work - reset RunState.numUnchangedTrackAttempts",
			"numProvisionedPVs", len(provisionedPVs))
		d.state.numUnchangedTrackAttempts = 0
	}
	numBound, err := volutil.FinalizeStaticBindingsForSelectedClaimsInWFFC(ctx, view)
	if err != nil {
		return err
	}
	if numBound > 0 {
		log.V(3).Info("Reset RunState.numUnchangedTrackAttempts since BindClaimsAndVolumesWithNonNilClaimRefs performed work", "numBound", numBound)
		d.state.numUnchangedTrackAttempts = 0
	}
	return nil
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
	return nil
}
