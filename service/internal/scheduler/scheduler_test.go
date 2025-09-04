package scheduler

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	mkserver "github.com/gardener/scaling-advisor/minkapi/server"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"os"
	"testing"
	"time"
)

var state suiteState

type suiteState struct {
	ctx             context.Context
	cancel          context.CancelFunc
	app             *mkapi.App
	nodeA           corev1.Node
	podA            corev1.Pod
	clientFacades   commontypes.ClientFacades
	schedulerHandle api.SchedulerHandle
	dynClient       dynamic.Interface
}

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
	<-time.After(5 * time.Second) // TODO: replace with better approach.
	evList := state.app.Server.GetBaseView().GetEventSink().List()
	if len(evList) == 0 {
		t.Fatalf("got no evList, want at least one")
		return
	}
	t.Logf("got numEvents: %d", len(evList))
	bindingEvent := evList[0]
	t.Logf("binding event note: %q", bindingEvent.Note)
	if bindingEvent.Action != "Binding" {
		t.Errorf("got event type %v, want %v", bindingEvent.Type, "Binding")
	}
	if bindingEvent.Reason != "Scheduled" {
		t.Errorf("got event reason %v, want %v", bindingEvent.Reason, "Scheduled")
	}
}

func initSuite(ctx context.Context) error {
	var err error
	var exitCode int

	app, exitCode := mkserver.LaunchApp(ctx)
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	<-time.After(1 * time.Second) // give some time for startup

	state.app = &app
	state.ctx, state.cancel = app.Ctx, app.Cancel
	log := logr.FromContextOrDiscard(state.ctx)
	state.clientFacades, err = clientutil.CreateNetworkClientFacades(log, app.Server.GetBaseView().GetKubeConfigPath(), mkapi.DefaultWatchTimeout)
	if err != nil {
		return err
	}

	launcher, err := NewLauncher("/tmp/minkapi-kube-scheduler-config.yaml", 1)
	if err != nil {
		return err
	}
	params := &api.SchedulerLaunchParams{
		ClientFacades: state.clientFacades,
		EventSink:     app.Server.GetBaseView().GetEventSink(),
	}
	state.schedulerHandle, err = launcher.Launch(state.ctx, params)
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
