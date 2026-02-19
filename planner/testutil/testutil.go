package testutil

import (
	"context"
	"encoding/json"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/view"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/planner/scheduler"
	"github.com/gardener/scaling-advisor/planner/weights"
	pricingtestutil "github.com/gardener/scaling-advisor/pricing/testutil"
	"github.com/gardener/scaling-advisor/samples"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"
)

const DefaultPlannerTestVerbosity = 5
const DefaultPlannerTestTimeout = 30 * time.Second

// Args represents the common test args for the scale-out unit-tests of the ScalingPlanner
type Args struct {
	NumUnscheduledPerResourceCategory map[samples.ResourceCategory]int
	PoolCategory                      samples.PoolCategory
	SimulatorStrategy                 commontypes.SimulatorStrategy
	NodeScoringStrategy               commontypes.NodeScoringStrategy
	AdviceGenerationMode              commontypes.ScalingAdviceGenerationMode
	Timeout                           time.Duration
	PVCNames                          []string
	PlannerFactory                    plannerapi.ScalingPlannerFactory
	VolumeBindingMode                 storagev1.VolumeBindingMode
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
	if len(args.NumUnscheduledPerResourceCategory) == 0 {
		t.Fatal("args.NumUnscheduledPerResourceCategory mandatory")
		return
	}
	testGenDir, ok := commontestutil.CreateTestGenDir(t)
	if !ok {
		return
	}
	var err error
	testData.RunContext, planner, ok = createTestScalingPlanner(t, testGenDir, args.PlannerFactory, args.Timeout)
	if !ok {
		return
	}
	testData.Request.CreationTime = time.Now()
	testData.Request.DiagnosticVerbosity = DefaultPlannerTestVerbosity
	testData.Request.ID = t.Name()
	if args.NodeScoringStrategy != "" {
		testData.Request.ScoringStrategy = args.NodeScoringStrategy
	} else {
		testData.Request.ScoringStrategy = commontypes.NodeScoringStrategyLeastCost
	}
	if args.SimulatorStrategy != "" {
		testData.Request.SimulatorStrategy = args.SimulatorStrategy
	} else {
		testData.Request.SimulatorStrategy = commontypes.SimulatorStrategySingleNodeMultiSim
	}
	if args.AdviceGenerationMode != "" {
		testData.Request.AdviceGenerationMode = args.AdviceGenerationMode
	} else {
		testData.Request.AdviceGenerationMode = commontypes.ScalingAdviceGenerationModeAllAtOnce
	}
	if args.VolumeBindingMode == "" {
		args.VolumeBindingMode = storagev1.VolumeBindingImmediate
	}
	testData.Request.Constraint, err = samples.LoadBasicScalingConstraints(args.PoolCategory)
	if err != nil {
		t.Fatal(err)
		return
	}
	var pods []corev1.Pod
	for c, n := range args.NumUnscheduledPerResourceCategory {
		pods, _, err = samples.GenerateSimplePodsForResourceCategory(c, n, samples.SimplePodGenInput{
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
	if len(args.PVCNames) > 0 {
		var (
			sc   storagev1.StorageClass
			pvcs []corev1.PersistentVolumeClaim
			pvs  []corev1.PersistentVolume
			pv   corev1.PersistentVolume
			pvc  corev1.PersistentVolumeClaim
		)
		sc, _, err = samples.GenerateStorageClass(testGenDir, commontypes.CloudProviderAWS, "default", args.VolumeBindingMode)
		if err != nil {
			t.Fatalf("failed to generate storage class %q: %v", "default", err)
			return
		}
		testData.Request.Snapshot.StorageClasses = append(testData.Request.Snapshot.StorageClasses, sc)
		volCommon := samples.VolCommon{
			GenDir:  testGenDir,
			Storage: resource.MustParse("1Gi"),
		}
		pvcs, _, err = samples.GeneratePersistentVolumeClaims(samples.SimplePVCGenInput{
			VolCommon: volCommon,
			Names:     args.PVCNames,
		})
		if err != nil {
			t.Fatalf("failed to generate pvcs: %v", err)
			return
		}
		for _, pvc = range pvcs {
			testData.Request.Snapshot.PVCs = append(testData.Request.Snapshot.PVCs, volutil.AsPVCInfo(pvc))
		}
		pvs, _, err = samples.GeneratePersistentVolumes(samples.SimplePVGenInput{
			VolCommon: volCommon,
			Zone:      testData.Request.Constraint.Spec.NodePools[0].AvailabilityZones[0],
			PVCNames:  args.PVCNames,
		})
		for _, pv = range pvs {
			testData.Request.Snapshot.PVs = append(testData.Request.Snapshot.PVs, volutil.AsPVInfo(pv))
		}
	}
	for _, pool := range testData.Request.Constraint.Spec.NodePools {
		for _, nt := range pool.NodeTemplates {
			for _, az := range pool.AvailabilityZones {
				testData.NodePlacements = append(testData.NodePlacements, sacorev1alpha1.NodePlacement{
					NodePoolName:     pool.Name,
					NodeTemplateName: nt.Name,
					InstanceType:     nt.InstanceType,
					Region:           pool.Region,
					AvailabilityZone: az,
				})
			}
		}
	}
	data, err := json.Marshal(testData.Request)
	if err != nil {
		t.Fatal("failed to marshal request:", err)
		return
	}
	reqJsonPath := path.Join(testGenDir, "request.json")
	err = os.WriteFile(reqJsonPath, data, 0644)
	if err != nil {
		t.Fatal("failed to write reqJsonPath", reqJsonPath, err)
		return
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

func createTestScalingPlanner(t *testing.T, traceDir string, factory plannerapi.ScalingPlannerFactory, duration time.Duration) (runCtx context.Context, planr plannerapi.ScalingPlanner, ok bool) {
	var err error
	defer func() {
		if err != nil {
			ok = false
			t.Errorf("failed to create test planner for test %q: %v", t.Name(), err)
			return
		}
	}()
	if duration == 0 {
		duration = DefaultPlannerTestTimeout
	}
	runCtx = testutil.NewTestContext(t, duration, DefaultPlannerTestVerbosity)
	pricingAccess, err := pricingtestutil.GetInstancePricingAccessForTop20AWSInstanceTypes()
	if err != nil {
		return
	}
	weightsFn := weights.GetDefaultWeightsFn()
	viewAccess, err := view.NewAccess(runCtx, &minkapi.ViewArgs{
		Name:   minkapi.DefaultBasePrefix,
		Scheme: typeinfo.SupportedScheme,
		WatchConfig: minkapi.WatchConfig{
			QueueSize: minkapi.DefaultWatchQueueSize,
			Timeout:   minkapi.DefaultWatchTimeout,
		},
	})
	if err != nil {
		return
	}

	schedulerConfigBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		return
	}
	simulatorConfig := plannerapi.SimulatorConfig{
		MaxParallelSimulations: plannerapi.DefaultMaxParallelSimulations,
		TrackPollInterval:      plannerapi.DefaultTrackPollInterval,
	}
	schedulerLauncher, err := scheduler.NewLauncherFromConfig(schedulerConfigBytes, simulatorConfig.MaxParallelSimulations)
	if err != nil {
		return
	}

	scalePlannerArgs := plannerapi.ScalingPlannerArgs{
		ViewAccess:        viewAccess,
		ResourceWeigher:   weightsFn,
		PricingAccess:     pricingAccess,
		SchedulerLauncher: schedulerLauncher,
		SimulatorConfig:   simulatorConfig,
		TraceDir:          traceDir,
	}
	planr, ok = factory(scalePlannerArgs), true
	return
}
