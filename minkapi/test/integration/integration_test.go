//go:build integration

package integration

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	"github.com/gardener/scaling-advisor/minkapi/server"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	"os"
	"testing"
	"time"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/minkapi/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var state suiteState

type suiteState struct {
	app           api.App
	nodeA         corev1.Node
	podA          corev1.Pod
	clientFacades commontypes.ClientFacades
}

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutdown.
func TestMain(m *testing.M) {
	err := initSuite(context.Background())
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
}

func TestWatchPods(t *testing.T) {
	var h eventsHolder
	client := state.clientFacades.Client
	watcher, err := client.CoreV1().Pods("").Watch(t.Context(), metav1.ListOptions{Watch: true})
	if err != nil {
		t.Fatalf("failed to create pods watcher: %v", err)
		return
	}
	defer watcher.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		listObjects(ctx, t, watcher.ResultChan(), &h)
	}()

	createdPod, err := client.CoreV1().Pods("").Create(ctx, &state.podA, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create podA: %v", err)
		return
	}
	t.Logf("Created podA with name %q", createdPod.Name)
	<-time.After(2 * time.Second)
	cancel()

	t.Logf("got numEvents: %d", len(h.events))
	if len(h.events) == 0 {
		t.Fatalf("got no events, want at least one")
		return
	}

	if h.events[0].Type != watch.Added {
		t.Errorf("got event type %v, want %v", h.events[0].Type, watch.Added)
	}
}

func listObjects(ctx context.Context, t *testing.T, eventCh <-chan watch.Event, h *eventsHolder) {
	t.Logf("Iterating eventCh: %v", eventCh)
	count := 0
outer:
	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				break outer
			}
			count++
			mo, err := meta.Accessor(ev.Object)
			if err != nil {
				t.Logf("received #%d event, Type: %s, Object: %v", count, ev.Type, ev.Object)
				continue
			}
			objFullName := objutil.CacheName(mo)
			objType, err := meta.TypeAccessor(ev.Object)
			if err != nil {
				t.Fatalf("failed to get TypeAccessor for event object %q: %v", objFullName, err)
				return
			}
			corev1.SchemeGroupVersion.WithKind(objType.GetKind()).GroupVersion().WithKind(objType.GetKind())
			t.Logf("received #%d event, Type: %s, ObjectName: %s", count, ev.Type, objFullName)
			h.events = append(h.events, ev)
		case <-ctx.Done():
			break outer
		}
	}
	t.Log("listObjects done")
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

func initSuite(ctx context.Context) error {
	var err error
	var exitCode int

	state.app, exitCode = server.LaunchApp(ctx)
	if exitCode != commoncli.ExitSuccess {
		os.Exit(exitCode)
	}
	<-time.After(1 * time.Second) // give minmal time for startup

	state.clientFacades, err = state.app.Server.GetBaseView().GetClientFacades()
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
	state.app.Cancel()
	_ = server.ShutdownApp(&state.app)
}
