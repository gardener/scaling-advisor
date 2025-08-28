package scheduler

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/testutil"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	mkcli "github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/service/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"os"
	"testing"
	"time"
)

var state suiteState

type suiteState struct {
	kapiApp         *mkcli.App
	nodeA           corev1.Node
	podA            corev1.Pod
	clientFacades   commontypes.ClientFacades
	schedulerHandle api.SchedulerHandle
	dynClient       dynamic.Interface
}

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutdown.
func TestMain(m *testing.M) {
	err := initSuite()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize suite state: %v\n", err)
		os.Exit(commoncli.ExitErrStart)
	}
	// Run integration tests
	exitCode := m.Run()
	shutdownSuite()
	os.Exit(exitCode)

}

func TestPodNodeAssignment(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := state.clientFacades.Client

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
	<-time.After(10 * time.Second)
	t.Logf("events  = %v", state.kapiApp.Service.GetBaseView().GetEventSink().List())
}

func initSuite() error {
	var err error

	app, exitCode := mkcli.LaunchApp()
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	defer app.Cancel()
	<-time.After(1 * time.Second) // give some time for startup

	state.kapiApp = &app
	state.clientFacades, err = app.Service.GetBaseView().GetClientFacades(mkapi.NetworkClient)
	if err != nil {
		return err
	}

	launcher, err := NewLauncher("/tmp/minkapi-kube-scheduler-config.yaml", 1)
	if err != nil {
		return err
	}
	params := &api.SchedulerLaunchParams{
		ClientFacades: state.clientFacades,
		EventSink:     app.Service.GetBaseView().GetEventSink(),
	}
	state.schedulerHandle, err = launcher.Launch(app.Ctx, params)
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
	exitCode := mkcli.ShutdownApp(state.kapiApp)
	if exitCode != commoncli.ExitSuccess {
	}
	os.Exit(exitCode)
}
