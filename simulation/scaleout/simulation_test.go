// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package scaleout_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	commontestutil "github.com/gardener/scaling-advisor/common/testutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/gardener/scaling-advisor/samples"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	"github.com/gardener/scaling-advisor/simulation/scaleout"
	"github.com/gardener/scaling-advisor/simulation/scheduler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDefaultSimulation_PodScheduledOnScaleOutNode verifies that a pod whose resource
// requests exceed the existing node's free capacity gets bound to the newly created
// scale-out node by the embedded kube-scheduler.
//
// Setup:
//   - Existing node: m5.large (1920m CPU / ~6.5Gi mem) already present in the sandbox view.
//   - Unscheduled pod: requests 3 CPU — too large for the existing node.
//   - Scale-out node template: m5.xlarge (4 CPU / 16Gi) — fits the pod.
//
// Expected: SimulationResult.NodePodAssignments contains the pod on the scale-out node,
// with no leftover unscheduled pods.
func TestDefaultSimulation_PodScheduledOnScaleOutNode(t *testing.T) {
	view, launcher, ctx := setup(t, "sim-scaleout")

	// Add a pod that is too large for the existing node (existing node has 1920m CPU allocatable).
	facades, err := view.GetClientFacades(ctx, minkapi.ClientAccessModeInMemory)
	if err != nil {
		t.Fatalf("GetClientFacades: %v", err)
	}
	pod := commontestutil.NewTestPod("big-pod", "bin-packing-scheduler", "3", "1Gi")
	if _, err = facades.Client.CoreV1().Pods(metav1.NamespaceDefault).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	sim, err := scaleout.NewDefault(simulationapi.SimulationArgs{
		Name:              "test-scaleout",
		SchedulerLauncher: launcher,
		RunCounter:        &atomic.Uint32{},
		Strategy:          apicommon.SimulatorStrategySingleNodeMultiSim,
		NodeTemplates:     []simulationapi.ScaleOutNodeTemplate{scaleOutNodeTemplate()},
		Config:            testSimConfig,
	})
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}

	result, err := sim.Run(ctx, view)
	if err != nil {
		t.Fatalf("simulation.Run: %v", err)
	}

	if len(result.LeftoverUnscheduledPods) != 0 {
		t.Errorf("expected no leftover unscheduled pods, got %v", result.LeftoverUnscheduledPods)
	}
	if len(result.NodePodAssignments) == 0 {
		t.Errorf("expected at least one NodePodAssignment on the scale-out node, got none")
	}
	if len(result.ScaleOutItems) == 0 {
		t.Errorf("expected at least one ScaleOutItem, got none")
	}
	wantPlacement := sacorev1alpha1.NodePlacement{
		PoolName:         testPoolName,
		TemplateName:     testTemplate,
		Region:           testRegion,
		AvailabilityZone: testZone,
		InstanceType:     testInstanceType,
	}
	found := false
	for _, item := range result.ScaleOutItems {
		if item.NodePlacement == wantPlacement && item.Delta > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ScaleOutItem with placement %+v and delta>0, got %+v", wantPlacement, result.ScaleOutItems)
	}
}

// TestDefaultSimulation_PodScheduledOnExistingNode verifies that a pod whose resource
// requests fit the existing node is bound there by the scheduler. The scale-out node is
// still created (the simulation always creates it), but the pod lands on the existing node,
// so NodePodAssignments on the scale-out node is empty and OtherNodePodAssignments is
// populated instead.
//
// Setup:
//   - Existing node: m5.large (1920m CPU / ~6.5Gi mem).
//   - Unscheduled pod: requests 100m CPU / 128Mi — fits the existing node easily.
//   - Scale-out node template: m5.xlarge.
//
// Expected: result.OtherNodePodAssignments contains the pod, result.NodePodAssignments
// is empty (scale-out node got no pods), no leftover unscheduled pods.
func TestDefaultSimulation_PodScheduledOnExistingNode(t *testing.T) {
	view, launcher, ctx := setup(t, "sim-existing")

	// Small pod — fits the existing m5.large comfortably.
	facades, err := view.GetClientFacades(ctx, minkapi.ClientAccessModeInMemory)
	if err != nil {
		t.Fatalf("GetClientFacades: %v", err)
	}
	pod := commontestutil.NewTestPod("small-pod", "bin-packing-scheduler", "100m", "128Mi")
	if _, err = facades.Client.CoreV1().Pods(metav1.NamespaceDefault).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	sim, err := scaleout.NewDefault(simulationapi.SimulationArgs{
		Name:              "test-existing-node",
		SchedulerLauncher: launcher,
		RunCounter:        &atomic.Uint32{},
		Strategy:          apicommon.SimulatorStrategySingleNodeMultiSim,
		NodeTemplates:     []simulationapi.ScaleOutNodeTemplate{scaleOutNodeTemplate()},
		Config:            testSimConfig,
	})
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}

	result, err := sim.Run(ctx, view)
	if err != nil {
		t.Fatalf("simulation.Run: %v", err)
	}

	if len(result.LeftoverUnscheduledPods) != 0 {
		t.Errorf("expected no leftover unscheduled pods, got %v", result.LeftoverUnscheduledPods)
	}
	if len(result.NodePodAssignments) != 0 {
		t.Errorf("expected no NodePodAssignments on the scale-out node, got %v", result.NodePodAssignments)
	}
	if len(result.OtherNodePodAssignments) == 0 {
		t.Errorf("expected pod to appear in OtherNodePodAssignments (scheduled on existing node), got none")
	}
}

const (
	testTimeout      = 30 * time.Second
	testRegion       = "eu-west-1"
	testZone         = "eu-west-1a"
	testPoolName     = "test-pool"
	testTemplate     = "m5.xlarge"
	testInstanceType = "m5.xlarge"
)

var (
	testSimConfig = simulationapi.SimulationConfig{
		TrackPollInterval:         plannerapi.DefaultTrackPollInterval,
		MaxUnchangedTrackAttempts: 10,
	}
)

// setup launches a real minkapi app, builds a scheduler launcher from the embedded
// bin-packing config, and returns a sandbox view seeded with the test node from
// testdata. sandboxName must be unique per test to ensure view isolation.
// ShutdownApp is registered via t.Cleanup — callers do not need a defer.
func setup(t *testing.T, sandboxName string) (view minkapi.View, launcher simulationapi.SchedulerLauncher, ctx context.Context) {
	t.Helper()
	ctx = commontestutil.NewTestContext(t, testTimeout, 1)

	app, _, err := cli.LaunchApp(ctx)
	if err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
	t.Cleanup(func() { cli.ShutdownApp(&app) })

	configBytes, err := samples.LoadBinPackingSchedulerConfig()
	if err != nil {
		t.Fatalf("LoadBinPackingSchedulerConfig: %v", err)
	}
	launcher, err = scheduler.NewLauncherFromConfig(configBytes, plannerapi.DefaultMaxParallelSimulations)
	if err != nil {
		t.Fatalf("NewLauncherFromConfig: %v", err)
	}

	view, err = app.Server.GetSandboxView(ctx, sandboxName)
	if err != nil {
		t.Fatalf("GetSandboxView: %v", err)
	}
	nodes, err := commontestutil.LoadTestNodes()
	if err != nil {
		t.Fatalf("LoadTestNodes: %v", err)
	}
	facades, err := view.GetClientFacades(ctx, minkapi.ClientAccessModeInMemory)
	if err != nil {
		t.Fatalf("GetClientFacades: %v", err)
	}
	if _, err = facades.Client.CoreV1().Nodes().Create(ctx, &nodes[0], metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed existing node: %v", err)
	}
	return
}

// scaleOutNodeTemplate builds a single ScaleOutNodeTemplate for an m5.xlarge in eu-west-1a.
// An m5.xlarge has 4 vCPU and 16 Gi memory.
func scaleOutNodeTemplate() simulationapi.ScaleOutNodeTemplate {
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
		corev1.ResourcePods:   resource.MustParse("110"),
	}
	return simulationapi.ScaleOutNodeTemplate{
		NodePlacement: sacorev1alpha1.NodePlacement{
			PoolName:         testPoolName,
			TemplateName:     testTemplate,
			Region:           testRegion,
			AvailabilityZone: testZone,
			InstanceType:     testInstanceType,
		},
		Architecture: "amd64",
		BaseResources: capacity,
		GroupKey:     simulationapi.GroupKey{NodePoolPriority: 1, NodeTemplatePriority: 1},
	}
}
