package testutil

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gardener/scaling-advisor/planner/scheduler"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/view"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
	gocmp "github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

const (
	// DefaultPlannerTestVerbosity indicates the default verbosity for the unit tests that construct the ScalingPlanner.
	DefaultPlannerTestVerbosity = 1
	// DefaultPlannerRunTestTimeout sets the default timeout for unit tests that construct the ScalingPlanner.
	DefaultPlannerRunTestTimeout = 30 * time.Second
)

// Args represents the common test args for the scale-out unit-tests of the ScalingPlanner
type Args struct {
	Factories                           plannerapi.Factories
	NumUnscheduledPodsPerResourcePreset map[samples.ResourcePreset]int
	PoolPreset                          samples.PoolPreset
	SimulatorStrategy                   commontypes.SimulatorStrategy
	NodeScoringStrategy                 commontypes.NodeScoringStrategy
	AdviceGenerationMode                commontypes.ScalingAdviceGenerationMode
	// PoolZones specifies the availability zones for each pool given in order for the NodePools of the PoolPreset.
	// If nil, defaults to the PoolPreset zone.
	PoolZones   [][]string
	VolGenInput samples.VolGenInput
	Timeout     time.Duration
}

// Data holds all the common test data necessary for carrying out the scale-out unit-tests of the ScalingPlanner and asserting conditions
// It also holds the generated test object YAML paths.
type Data struct {
	GenDir         string
	RunContext     context.Context
	NodePlacements []sacorev1alpha1.NodePlacement
	Request        plannerapi.Request
}

// CreateTestPlannerAndTestData creates a ScalingPlanner suitable for unit tests and test Data for the given Args.
func CreateTestPlannerAndTestData(t *testing.T, args Args) (planner plannerapi.ScalingPlanner, testData Data, ok bool) {
	var (
		podGenOutput samples.PodGenOutput
		err          error
	)
	testData.Request.DiagnosticVerbosity = ioutil.GetEnvAsUint32("VERBOSITY", DefaultPlannerTestVerbosity)
	if ok = testData.validateAndFillDefaults(t, &args); !ok {
		return
	}
	if testData.GenDir, ok = commontestutil.CreateTestGenDir(t); !ok {
		return
	}
	constraintGenInput := samples.ConstraintGenInput{GenDir: testData.GenDir, PoolPreset: args.PoolPreset, PoolZones: args.PoolZones}
	constraintGenOutput, err := samples.GenScalingConstraints(constraintGenInput)
	if err != nil {
		t.Fatalf("failed to generate constraints: %v", err)
		return
	}
	testData.Request.Constraint = &constraintGenOutput.Constraint
	for category, num := range args.NumUnscheduledPodsPerResourcePreset {
		podGenOutput, err = samples.GenerateSimplePodsForResourcePreset(category, num, samples.PodGenInput{
			GenDir:        testData.GenDir,
			Name:          string(category),
			SchedulerName: "bin-packing-scheduler",
			PVCNames:      args.VolGenInput.PVCNames,
		})
		if err != nil {
			t.Fatalf("failed to generate simple pods for resource category %s: %v", category, err)
			return
		}
		testData.Request.Snapshot.Pods = append(testData.Request.Snapshot.Pods, podutil.PodInfosFromCoreV1Pods(podGenOutput.Pods)...)
	}
	if len(args.VolGenInput.PVCNames) > 0 {
		if args.VolGenInput.PVZones == nil {
			// If PVZones is not specified, default to all available zones across all NodePools of the ScalingConstraint
			args.VolGenInput.PVZones = testData.Request.Constraint.Spec.GetAllAvailabilityZones()
		}
		if ok = GenAndFillStorageAndVolumeObjects(t, testData.GenDir, args.VolGenInput, &testData.Request.Snapshot); !ok {
			return
		}
	}
	testData.NodePlacements = getAllNodePlacements(testData.Request.Constraint.Spec)
	data, err := json.Marshal(testData.Request)
	if err != nil {
		t.Fatal("failed to marshal request:", err)
		return
	}
	reqJsonPath := path.Join(testData.GenDir, "request.json")
	if err = os.WriteFile(reqJsonPath, data, 0600); err != nil {
		t.Fatal("failed to write request.json:", err)
		return
	}
	if testData.RunContext, planner, ok = CreateTestScalingPlanner(t, args, testData.GenDir, testData.Request.DiagnosticVerbosity); !ok {
		return
	}
	ok = true
	return
}

// GenAndFillStorageAndVolumeObjects uses the given VolGenInput to generate StorageClasses, PVC's and PV's and also
// fills the given ClusterSnapshot with the generated objects.
func GenAndFillStorageAndVolumeObjects(t *testing.T, testGenDir string, volGenInput samples.VolGenInput, snap *plannerapi.ClusterSnapshot) (ok bool) {
	var (
		err       error
		sc        storagev1.StorageClass
		volGenOut samples.VolGenOutput
		pv        corev1.PersistentVolume
		pvc       corev1.PersistentVolumeClaim
	)
	if err = volGenInput.ValidateAndFillDefaults(); err != nil {
		t.Fatalf("failed to validate input: %v", err)
		return
	}
	if sc, _, err = samples.GenerateDefaultStorageClass(testGenDir, volGenInput.Provider, "default", volGenInput.VolumeBindingMode); err != nil {
		t.Fatalf("failed to generate storage class %q: %v", "default", err)
		return
	}
	snap.StorageClasses = append(snap.StorageClasses, sc)

	if volGenOut, err = samples.GeneratePersistentVolumeClaims(testGenDir, volGenInput); err != nil {
		return
	}
	for _, pvc = range volGenOut.PVCs {
		snap.PVCs = append(snap.PVCs, volutil.AsPVCInfo(pvc))
	}

	if volGenInput.GeneratePV {
		if volGenOut, err = samples.GeneratePersistentVolumes(testGenDir, volGenInput); err != nil {
			return
		}
		for _, pv = range volGenOut.PVs {
			snap.PVs = append(snap.PVs, volutil.AsPVInfo(pv))
		}
	}
	ok = true
	return
}

// AssertExactScaleOutPlan asserts that the wanted ScaleOutPlan matches the gotten ScaleOutPlan
func AssertExactScaleOutPlan(t *testing.T, want, got *sacorev1alpha1.ScaleOutPlan) bool {
	if got == nil {
		t.Fatalf("got nil ScaleOutPlan, want not nil ScaleOutPlan")
		return false
	}
	slices.SortFunc(want.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.PoolName, b.PoolName)
	})
	slices.SortFunc(got.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.PoolName, b.PoolName)
	})
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Errorf("ScaleOutPlan mismatch (-want +got):\n%s", diff)
		return false
	}
	return true
}

// ObtainAndAssertScaleOutPlan executes the given planner with the context and request within testData, obtains the plannerapi.Response
// logs the same, and asserts that the embedded ScaleOutPlan within the Response matches the wanted ScaleOutPlan.
// Returns true if all assertions succeeded or false if assertion failed or on any error.
func ObtainAndAssertScaleOutPlan(t *testing.T, planner plannerapi.ScalingPlanner, testData *Data, wantPlan *sacorev1alpha1.ScaleOutPlan) bool {
	response, ok := ObtainPlannerResponse(t, planner, testData)
	if !ok {
		return false
	}
	return AssertExactScaleOutPlan(t, wantPlan, response.ScaleOutPlan)
}

// ObtainPlannerResponse executes the given planner with the context and request within testData, obtains the
// plannerapi.Response logs and returns the same if successful. Returns false in case of any error and also fails the
// test state.
func ObtainPlannerResponse(t *testing.T, planner plannerapi.ScalingPlanner, testData *Data) (response plannerapi.Response, ok bool) {
	responseCh := planner.Plan(testData.RunContext, testData.Request)
	response = <-responseCh
	if response.Error != nil {
		t.Fatalf("failed to generate scale-out plan: %v", response.Error)
		return
	}
	planResultJson, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal Response: %v", err)
		return
	}
	t.Logf("Obtained plannerapi.Response %s", planResultJson)
	respJsonPath := path.Join(testData.GenDir, "response.json")
	if err = os.WriteFile(respJsonPath, planResultJson, 0600); err != nil {
		t.Fatal("failed to write response.json", err)
		return
	}
	ok = true
	return
}

// CreateTestScalingPlanner creates a ScalingPlanner for unit-tests with the given context timeout for the given provider,
// using traceDir for traces and object dumps and leveraging the given factories.
func CreateTestScalingPlanner(t *testing.T, args Args, traceDir string, verbosity uint32) (runCtx context.Context, planr plannerapi.ScalingPlanner, ok bool) {
	//t *testing.T, timeout time.Duration, provider commontypes.CloudProvider, traceDir string, factories plannerapi.Factories) (runCtx context.Context, planr plannerapi.ScalingPlanner, ok bool) {
	var err error
	defer func() {
		if err != nil {
			ok = false
			t.Errorf("failed to create test planner for test %q: %v", t.Name(), err)
			return
		}
	}()
	if args.Timeout == 0 {
		if os.Getenv("DEBUG") == "" {
			args.Timeout = DefaultPlannerRunTestTimeout
		} else {
			args.Timeout = DefaultPlannerRunTestTimeout * 10
		}
	}
	runCtx = commontestutil.NewTestContext(t, args.Timeout, int(verbosity))
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessForTop20AWSInstanceTypes()
	if err != nil {
		t.Fatalf("failed to get instance pricing access: %v", err)
		return
	}
	viewAccess, err := view.NewAccess(runCtx, &minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: minkapi.DefaultWatchQueueSize,
			Timeout:   minkapi.DefaultWatchTimeout,
		},
	})
	if err != nil {
		t.Fatalf("failed to create ViewAccess: %v", err)
		return
	}

	schedulerConfigBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		t.Fatalf("failed to load scheduler config: %v", err)
		return
	}
	var simulatorConfig plannerapi.SimulatorConfig
	if os.Getenv("DEBUG") == "" {
		simulatorConfig = plannerapi.SimulatorConfig{
			MaxParallelSimulations:    plannerapi.DefaultMaxParallelSimulations,
			TrackPollInterval:         plannerapi.DefaultTrackPollInterval,
			MaxUnchangedTrackAttempts: plannerapi.DefaultMaxUnchangedTrackAttempts,
		}
	} else { // allow comfortable time for debugging
		simulatorConfig = plannerapi.SimulatorConfig{
			MaxParallelSimulations:    plannerapi.DefaultMaxParallelSimulations,
			TrackPollInterval:         100 * plannerapi.DefaultTrackPollInterval,
			MaxUnchangedTrackAttempts: 10 * plannerapi.DefaultMaxUnchangedTrackAttempts,
		}
	}
	simulatorConfig.BindVolumeClaimsForImmediateMode = true
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		t.Fatalf("failed to create SchedulerLauncher: %v", err)
		return
	}
	storageMetaAccess := &testStorageMetaAccess{provider: args.VolGenInput.Provider}
	scalePlannerArgs := plannerapi.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   args.Factories.ResourceWeigher,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		StorageMetaAccess: storageMetaAccess,
		SimulatorConfig:   simulatorConfig,
		SimulatorFactory:  args.Factories.Simulator,
		SimulationFactory: args.Factories.Simulation,
		TraceDir:          traceDir,
	}
	planr, err = args.Factories.Planner.NewPlanner(scalePlannerArgs)
	if err != nil {
		t.Fatalf("failed to create ScalingPlanner from args %+v: %v", scalePlannerArgs, err)
	} else {
		ok = true
	}
	return
}

func (d *Data) validateAndFillDefaults(t *testing.T, args *Args) bool {
	if len(args.NumUnscheduledPodsPerResourcePreset) == 0 {
		t.Fatal("args.NumUnscheduledPodsPerResourcePreset mandatory")
		return false
	}
	d.Request.CreationTime = time.Now()
	if d.Request.DiagnosticVerbosity <= 0 {
		d.Request.DiagnosticVerbosity = DefaultPlannerTestVerbosity
	}
	d.Request.ID = t.Name()
	d.Request.ScoringStrategy = cmp.Or(args.NodeScoringStrategy, commontypes.NodeScoringStrategyLeastCost)
	args.VolGenInput.Provider = cmp.Or(args.VolGenInput.Provider, commontypes.CloudProviderAWS)
	d.Request.SimulatorStrategy = cmp.Or(args.SimulatorStrategy, commontypes.SimulatorStrategySingleNodeMultiSim)
	d.Request.AdviceGenerationMode = cmp.Or(args.AdviceGenerationMode, commontypes.ScalingAdviceGenerationModeAllAtOnce)
	return true
}

var _ plannerapi.StorageMetaAccess = (*testStorageMetaAccess)(nil)

type testStorageMetaAccess struct {
	provider commontypes.CloudProvider
}

func (s *testStorageMetaAccess) GetFallbackCSINodeSpec(instanceType string) (csiNodeSpec storagev1.CSINodeSpec, err error) {
	maxVolumes := samples.GetMaxAllocatableVolumes(s.provider, instanceType)
	csiNodeSpec.Drivers, err = samples.GetCSINodeDrivers(s.provider, maxVolumes)
	return
}

// getAllNodePlacements computes all the possible NodePlacements for the ScalingConstraintSpec.
func getAllNodePlacements(c sacorev1alpha1.ScalingConstraintSpec) (placements []sacorev1alpha1.NodePlacement) {
	placements = make([]sacorev1alpha1.NodePlacement, 0, len(c.NodePools)*len(c.GetAllAvailabilityZones()))
	for _, p := range c.NodePools {
		placements = append(placements, p.GetNodePlacements()...)
	}
	return placements
}
