package testutil

import (
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
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

// DefaultPlannerTestVerbosity indicates the default verbosity for the unit tests that construct the ScalingPlanner.
const DefaultPlannerTestVerbosity = 1

// DefaultPlannerTestTimeout sets the default timeout for unit tests that construct the ScalingPlanner.
const DefaultPlannerTestTimeout = 30 * time.Second

// Args represents the common test args for the scale-out unit-tests of the ScalingPlanner
type Args struct {
	Factories                         plannerapi.Factories
	NumUnscheduledPerResourceCategory map[samples.ResourcePreset]int
	PoolCategory                      samples.PoolCategory
	SimulatorStrategy                 commontypes.SimulatorStrategy
	NodeScoringStrategy               commontypes.NodeScoringStrategy
	AdviceGenerationMode              commontypes.ScalingAdviceGenerationMode
	VolumeBindingMode                 storagev1.VolumeBindingMode
	Provider                          commontypes.CloudProvider
	PVCNames                          []string
	Timeout                           time.Duration
}

// Data holds all the common test data necessary for carrying out the scale-out unit-tests of the ScalingPlanner and asserting conditions
type Data struct {
	RunContext     context.Context
	SnapshotPath   string
	NodePlacements []sacorev1alpha1.NodePlacement
	Request        plannerapi.Request
}

// CreateTestPlannerAndTestData creates a ScalingPlanner suitable for unit tests and test Data for the given Args.
func CreateTestPlannerAndTestData(t *testing.T, args Args) (planner plannerapi.ScalingPlanner, testData Data, ok bool) {
	var (
		testGenDir string
		pods       []corev1.Pod
		err        error
	)
	if ok = testData.validateAndFillDefaults(t, &args); !ok {
		return
	}
	if testGenDir, ok = commontestutil.CreateTestGenDir(t); !ok {
		return
	}
	if testData.Request.Constraint, err = samples.LoadBasicScalingConstraints(args.PoolCategory); err != nil {
		t.Fatalf("failed to load constraints: %v", err)
		return
	}
	for c, n := range args.NumUnscheduledPerResourceCategory {
		pods, _, err = samples.GenerateSimplePodsForResourceCategory(c, n, samples.PodGenInput{
			GenDir:        testGenDir,
			Name:          string(c),
			SchedulerName: "bin-packing-scheduler",
			PVCNames:      args.PVCNames,
		})
		if err != nil {
			t.Fatalf("failed to generate simple pods for resource category %s: %v", c, err)
			return
		}
		testData.Request.Snapshot.Pods = append(testData.Request.Snapshot.Pods, podutil.PodInfosFromCoreV1Pods(pods)...)
	}
	allZones := testData.Request.Constraint.Spec.GetAllAvailabilityZones()
	if len(args.PVCNames) > 0 {
		volGenInput := samples.StorageVolGenInput{
			Provider:          args.Provider,
			GenDir:            testGenDir,
			VolumeBindingMode: args.VolumeBindingMode,
			PVCNames:          args.PVCNames,
			PVZones:           allZones,
		}
		if ok = GenFillStorageAndVolumeObjects(t, testGenDir, volGenInput, &testData.Request.Snapshot); !ok {
			return
		}
	}
	testData.NodePlacements = testData.Request.Constraint.Spec.GetAllNodePlacements()
	data, err := json.Marshal(testData.Request)
	if err != nil {
		t.Fatal("failed to marshal request:", err)
		return
	}
	reqJsonPath := path.Join(testGenDir, "request.json")
	if err = os.WriteFile(reqJsonPath, data, 0600); err != nil {
		t.Fatal("failed to write request.json:", err)
		return
	}
	if testData.RunContext, planner, ok = CreateTestScalingPlanner(t, args.Timeout, args.Provider, testGenDir, args.Factories); !ok {
		return
	}
	ok = true
	return
}

// GenFillStorageAndVolumeObjects uses the given StorageVolGenInput to generate StorageClasses, PVC's and PV's and also
// populate them in given ClusterSnapshot.
func GenFillStorageAndVolumeObjects(t *testing.T, testGenDir string, volGenInput samples.StorageVolGenInput, snap *plannerapi.ClusterSnapshot) (ok bool) {
	var (
		err  error
		sc   storagev1.StorageClass
		pvcs []corev1.PersistentVolumeClaim
		pvs  []corev1.PersistentVolume
		pv   corev1.PersistentVolume
		pvc  corev1.PersistentVolumeClaim
	)
	if err = volGenInput.ValidateAndFillDefaults(); err != nil {
		return
	}
	if sc, _, err = samples.GenerateStorageClass(testGenDir, volGenInput.Provider, "default", volGenInput.VolumeBindingMode); err != nil {
		t.Fatalf("failed to generate storage class %q: %v", "default", err)
		return
	}
	snap.StorageClasses = append(snap.StorageClasses, sc)

	if pvcs, _, err = samples.GeneratePersistentVolumeClaims(volGenInput); err != nil {
		return
	}
	for _, pvc = range pvcs {
		snap.PVCs = append(snap.PVCs, volutil.AsPVCInfo(pvc))
	}

	if pvs, _, err = samples.GeneratePersistentVolumes(volGenInput); err != nil {
		return
	}
	for _, pv = range pvs {
		snap.PVs = append(snap.PVs, volutil.AsPVInfo(pv))
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
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	slices.SortFunc(got.Items, func(a, b sacorev1alpha1.ScaleOutItem) int {
		return strings.Compare(a.NodePoolName, b.NodePoolName)
	})
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ScaleOutPlan mismatch (-want +got):\n%s", diff)
		return false
	}
	return true
}

// ObtainAndAssertScaleOutPlan executes the given planner with the context and request within testData, obtains the plannerapi.Response
// logs the same, and asserts that the embedded ScaleOutPlan within the Response matches the wanted ScaleOutPlan.
// Returns true if all assertions succeeded or false if assertion failed or on any error.
func ObtainAndAssertScaleOutPlan(t *testing.T, planner plannerapi.ScalingPlanner, testData *Data, wantPlan *sacorev1alpha1.ScaleOutPlan) bool {
	responseCh := planner.Plan(testData.RunContext, testData.Request)
	response := <-responseCh
	if response.Error != nil {
		t.Fatalf("failed to generate scale-out plan: %v", response.Error)
		return false
	} else {
		planResultJson, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			t.Fatalf("failed to marshal Response: %v", err)
			return false
		}
		t.Logf("Obtained plannerapi.Response %s", planResultJson)
		return AssertExactScaleOutPlan(t, wantPlan, response.ScaleOutPlan)
	}
}

// CreateTestScalingPlanner creates a ScalingPlanner for unit-tests with the given context timeout for the given provider,
// using traceDir for traces and object dumps and leveraging the given factories.
func CreateTestScalingPlanner(t *testing.T, timeout time.Duration, provider commontypes.CloudProvider, traceDir string, factories plannerapi.Factories) (runCtx context.Context, planr plannerapi.ScalingPlanner, ok bool) {
	var err error
	defer func() {
		if err != nil {
			ok = false
			t.Errorf("failed to create test planner for test %q: %v", t.Name(), err)
			return
		}
	}()
	if timeout == 0 {
		timeout = DefaultPlannerTestTimeout
	}
	runCtx = commontestutil.NewTestContext(t, timeout, DefaultPlannerTestVerbosity)
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
	simulatorConfig := plannerapi.SimulatorConfig{
		MaxParallelSimulations: plannerapi.DefaultMaxParallelSimulations,
		TrackPollInterval:      plannerapi.DefaultTrackPollInterval,
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		t.Fatalf("failed to create SchedulerLauncher: %v", err)
		return
	}
	storageMetaAccess := &testStorageMetaAccess{provider: provider}
	scalePlannerArgs := plannerapi.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   factories.ResourceWeigher,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		StorageMetaAccess: storageMetaAccess,
		SimulatorConfig:   simulatorConfig,
		SimulatorFactory:  factories.Simulator,
		SimulationFactory: factories.Simulation,
		TraceDir:          traceDir,
	}
	planr, err = factories.Planner.NewPlanner(scalePlannerArgs)
	if err != nil {
		t.Fatalf("failed to create ScalingPlanner from args %+v: %v", scalePlannerArgs, err)
	} else {
		ok = true
	}
	return
}

func (d *Data) validateAndFillDefaults(t *testing.T, args *Args) bool {
	if len(args.NumUnscheduledPerResourceCategory) == 0 {
		t.Fatal("args.NumUnscheduledPerResourceCategory mandatory")
		return false
	}
	d.Request.CreationTime = time.Now()
	d.Request.DiagnosticVerbosity = DefaultPlannerTestVerbosity
	d.Request.ID = t.Name()
	if args.NodeScoringStrategy != "" {
		d.Request.ScoringStrategy = args.NodeScoringStrategy
	} else {
		d.Request.ScoringStrategy = commontypes.NodeScoringStrategyLeastCost
	}
	if args.Provider == "" {
		args.Provider = commontypes.CloudProviderAWS
	}
	if args.SimulatorStrategy != "" {
		d.Request.SimulatorStrategy = args.SimulatorStrategy
	} else {
		d.Request.SimulatorStrategy = commontypes.SimulatorStrategySingleNodeMultiSim
	}
	if args.AdviceGenerationMode != "" {
		d.Request.AdviceGenerationMode = args.AdviceGenerationMode
	} else {
		d.Request.AdviceGenerationMode = commontypes.ScalingAdviceGenerationModeAllAtOnce
	}
	if args.VolumeBindingMode == "" {
		args.VolumeBindingMode = storagev1.VolumeBindingImmediate
	}
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
