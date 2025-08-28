//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/common/testutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/watch"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var state suiteState

type suiteState struct {
	app           *cli.App
	nodeA         corev1.Node
	podA          corev1.Pod
	clientFacades commontypes.ClientFacades
}

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutsdown.
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

func TestBaseViewCreateGetNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodesFacade := state.clientFacades.Client.CoreV1().Nodes()

	t.Run("checkInitialNodeList", func(t *testing.T) {
		nodeList, err := nodesFacade.List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to list nodes: %w", err))
		}
		if len(nodeList.Items) != 0 {
			t.Errorf("len(nodeList)=%d, want %d", len(nodeList.Items), 0)
		}
	})

	t.Run("createGetNode", func(t *testing.T) {
		createdNode, err := nodesFacade.Create(ctx, &state.nodeA, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to create node: %w", err))
		}
		gotNode, err := nodesFacade.Get(ctx, createdNode.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(fmt.Errorf("failed to get node: %w", err))
		}
		checkNodeIsSame(t, gotNode, createdNode)
	})

}

type eventsHolder struct {
	events []watch.Event
	wg     sync.WaitGroup
}

func TestWatchPods(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var h eventsHolder
	client := state.clientFacades.Client
	watcher, err := client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{Watch: true})
	if err != nil {
		t.Fatalf("failed to create pods watcher: %v", err)
		return
	}
	defer watcher.Stop()

	h.wg.Add(1)
	go func() {
		listObjects(t, watcher, &h)
	}()
	createdPod, err := client.CoreV1().Pods("").Create(ctx, &state.podA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create podA: %v", err)
		return
	}
	t.Logf("Created podA with name %q", createdPod.Name)
	h.wg.Wait()

	t.Logf("got numEvents: %d", len(h.events))
	if len(h.events) == 0 {
		t.Fatalf("got no events, want at least one")
		return
	}
	for _, ev := range h.events {
		mo, err := meta.Accessor(ev.Object)
		if err != nil {
			t.Logf("WARN: Got event which is not a metav1.Object: %v", err)
			continue
		}
		t.Logf("got event-> Type: %v | Object Kind: %q, Object Name: %q", ev.Type, ev.Object.GetObjectKind().GroupVersionKind(), cache.NewObjectName(mo.GetNamespace(), mo.GetName()))
	}
	if h.events[0].Type != watch.Added {
		t.Errorf("got event type %v, want %v", h.events[0].Type, watch.Added)
	}
}

func listObjects(t *testing.T, watcher watch.Interface, h *eventsHolder) {
	watchCh := watcher.ResultChan()
	t.Logf("Iterating watchCh: %v", watchCh)
	for ev := range watchCh {
		h.events = append(h.events, ev)
		h.wg.Done()
	}
	return
}
func checkNodeIsSame(t *testing.T, got, want *corev1.Node) {
	t.Helper()
	if got.Name != want.Name {
		t.Errorf("got.Name=%s, want %s", got.Name, want.Name)
	}
	if got.Spec.ProviderID != want.Spec.ProviderID {
		t.Errorf("got.Spec.ProviderID=%s, want %s", got.Spec.ProviderID, want.Spec.ProviderID)
	}
}

func initSuite() error {
	var err error

	app, exitCode := cli.LaunchApp()
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	defer app.Cancel()
	<-time.After(1 * time.Second) // give some time for startup

	state.app = &app
	state.clientFacades, err = app.Service.GetBaseView().GetClientFacades(api.NetworkClient)
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
	exitCode := cli.ShutdownApp(state.app)
	if exitCode != commoncli.ExitSuccess {
	}
	os.Exit(exitCode)
}
