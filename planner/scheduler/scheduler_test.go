// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type suiteState struct {
	ctx             context.Context
	cancel          context.CancelFunc
	app             *mkapi.App
	nodeA           corev1.Node
	podA            corev1.Pod
	baseView        mkapi.View
	wamView         mkapi.View
	bamView         mkapi.View
	schedulerHandle svcapi.SchedulerHandle
}

var log = klog.NewKlogr()

func TestSingleSchedulerPodNodeAssignment(t *testing.T) {
	suite, err := initSuite(context.Background())
	if err != nil {
		t.Fatalf("failed to init suit: %v", err)
		return
	}
	defer shutdownSuite(&suite)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clientFacades, err := suite.baseView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		t.Fatalf("failed to get client facades: %v", err)
		return
	}
	client := clientFacades.Client

	createdNode, err := client.CoreV1().Nodes().Create(ctx, &suite.nodeA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create nodeA: %v", err)
		return
	}
	t.Logf("Created nodeA with name %q", createdNode.Name)

	createdPod, err := client.CoreV1().Pods("").Create(ctx, &suite.podA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create podA: %v", err)
		return
	}
	t.Logf("Created podA with name %q", createdPod.Name)
	<-time.After(6 * time.Second) // TODO: replace with better approach.
	evList := suite.app.Server.GetBaseView().GetEventSink().List()
	if len(evList) == 0 {
		t.Fatalf("got no evList, want at least one")
		return
	}
	t.Logf("got numEvents: %d", len(evList))
	for _, ev := range evList {
		t.Logf("got event| Type: %q, ReprotingController: %q, ReportingInstance: %q, Action: %q, Reason: %q, Regarding: %q, Note: %q",
			ev.Type, ev.ReportingController, ev.ReportingInstance, ev.Action, ev.Reason, ev.Regarding, ev.Note)
	}
	foundBinding := false
	for _, ev := range evList {
		if ev.Action == "Binding" {
			foundBinding = true
			if ev.Reason != "Scheduled" {
				t.Errorf("got event reason %v, want %v", ev.Reason, "Scheduled")
				return
			}
		}
	}
	if !foundBinding {
		t.Errorf("got no Binding event, want at least one")
	}
}

func initSuite(ctx context.Context) (suite suiteState, err error) {
	ctx = logr.NewContext(ctx, log)
	ctx = context.WithValue(ctx, commonconstants.VerbosityCtxKey, 1)
	app, _, err := cli.LaunchApp(ctx)
	if err != nil {
		return
	}

	// Wait for the MinKAPI server to fully initialize and create the config file
	configPath := "/tmp/minkapi-bin-packing-scheduler-config.yaml"
	maxWait := 30 * time.Second
	checkInterval := 500 * time.Millisecond

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(configPath); err == nil {
			// Config file exists, proceed
			break
		}
		time.Sleep(checkInterval)
	}

	// Final check that the config file exists
	if _, err = os.Stat(configPath); err != nil {
		err = fmt.Errorf("scheduler config file not found after waiting %v: %w", maxWait, err)
		return
	}

	suite.app = &app
	suite.ctx, suite.cancel = app.Ctx, app.Cancel
	suite.baseView = app.Server.GetBaseView()
	suite.wamView, err = app.Server.GetSandboxView(suite.ctx, "wam")
	if err != nil {
		return
	}
	suite.bamView, err = app.Server.GetSandboxView(suite.ctx, "bam")
	if err != nil {
		return
	}

	launcher, err := NewLauncher(configPath, 1)
	if err != nil {
		return
	}
	clientFacades, err := suite.baseView.GetClientFacades(suite.ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return
	}
	suite.schedulerHandle, err = launcher.Launch(suite.ctx, &svcapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     app.Server.GetBaseView().GetEventSink(),
	})
	if err != nil {
		return
	}
	nodes, err := testutil.LoadTestNodes()
	if err != nil {
		return
	}
	suite.nodeA = nodes[0]

	pods, err := testutil.LoadTestPods()
	if err != nil {
		return
	}
	suite.podA = pods[0]

	return
}

func shutdownSuite(state *suiteState) {
	err := state.schedulerHandle.Close()
	if err != nil {
		klog.Errorf("failed to close schedulerHandle: %v", err)
	}
	exitCode := cli.ShutdownApp(state.app)
	if exitCode != 0 {
		klog.Errorf("failed to shutdown minkapi api: %v, exitCode: %d", state.app, exitCode)
	}
}
