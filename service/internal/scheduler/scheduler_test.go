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

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/testutil"
	mkserver "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

var state suiteState

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
	dynClient       dynamic.Interface
}

var log = klog.NewKlogr()

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutdown.
func TestMain(m *testing.M) {
	err := initSuite(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize suite state: %v\n", err)
		os.Exit(commoncli.ExitErrStart)
	}
	defer state.cancel()
	// Run integration tests
	exitCode := m.Run()
	shutdownSuite()
	os.Exit(exitCode)

}

func TestSingleSchedulerPodNodeAssignment(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clientFacades, err := state.baseView.GetClientFacades()
	if err != nil {
		t.Fatalf("failed to get client facades: %v", err)
		return
	}
	client := clientFacades.Client

	createdNode, err := client.CoreV1().Nodes().Create(ctx, &state.nodeA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create nodeA: %v", err)
		return
	}
	t.Logf("Created nodeA with name %q", createdNode.Name)

	createdPod, err := client.CoreV1().Pods("").Create(ctx, &state.podA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create podA: %v", err)
		return
	}
	t.Logf("Created podA with name %q", createdPod.Name)
	<-time.After(6 * time.Second) // TODO: replace with better approach.
	evList := state.app.Server.GetBaseView().GetEventSink().List()
	if len(evList) == 0 {
		t.Fatalf("got no evList, want at least one")
		return
	}
	t.Logf("got numEvents: %d", len(evList))
	for _, ev := range evList {
		t.Logf("got event| Type: %q, ReprotingController: %q, ReportingInstance: %q, Action: %q, Reason: %q, Regarding: %q, Note: %q",
			ev.Type, ev.ReportingController, ev.ReportingInstance, ev.Action, ev.Reason, ev.Regarding, ev.Note)
	}
	bindingEvent := evList[0]
	t.Logf("binding event note: %q", bindingEvent.Note)
	if bindingEvent.Action != "Binding" {
		t.Errorf("got event Action %v, want %v", bindingEvent.Action, "Binding")
	}
	if bindingEvent.Reason != "Scheduled" {
		t.Errorf("got event reason %v, want %v", bindingEvent.Reason, "Scheduled")
	}
}

func initSuite(ctx context.Context) error {
	var err error
	var exitCode int
	ctx = logr.NewContext(ctx, log)

	app, exitCode := mkserver.LaunchApp(ctx)
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}

	// Wait for the MinKAPI server to fully initialize and create the config file
	configPath := "/tmp/minkapi-kube-scheduler-config.yaml"
	maxWait := 30 * time.Second
	checkInterval := 500 * time.Millisecond

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(configPath); err == nil {
			// Config file exists, proceed
			break
		}
		time.Sleep(checkInterval)
	}

	// Final check that the config file exists
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("scheduler config file not found after waiting %v: %w", maxWait, err)
	}

	state.app = &app
	state.ctx, state.cancel = app.Ctx, app.Cancel
	state.baseView = app.Server.GetBaseView()
	state.wamView, err = app.Server.GetSandboxView(log, "wam")
	if err != nil {
		return err
	}
	state.bamView, err = app.Server.GetSandboxView(log, "bam")
	if err != nil {
		return err
	}

	launcher, err := NewLauncher(configPath, 1)
	if err != nil {
		return err
	}
	clientFacades, err := state.baseView.GetClientFacades()
	if err != nil {
		return err
	}
	state.schedulerHandle, err = launcher.Launch(state.ctx, &svcapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     app.Server.GetBaseView().GetEventSink(),
	})
	if err != nil {
		return err
	}
	nodes, err := testutil.LoadTestNodes()
	if err != nil {
		return err
	}
	state.nodeA = nodes[0]

	pods, err := testutil.LoadTestPods()
	if err != nil {
		return err
	}
	state.podA = pods[0]

	return nil
}

func shutdownSuite() {
	state.schedulerHandle.Stop()
	_ = mkserver.ShutdownApp(state.app)
}
