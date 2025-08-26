//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/cli"
	"github.com/gardener/scaling-advisor/minkapi/server"

	commoncli "github.com/gardener/scaling-advisor/common/cli"
	"github.com/gardener/scaling-advisor/common/clientutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

var state suiteState

type suiteState struct {
	apiServer api.Server
	nodeA     corev1.Node
	podA      corev1.Pod
	client    kubernetes.Interface
	dynClient dynamic.Interface
}

// TestMain sets up the MinKAPI server once for all tests in this package, runs tests and then shutsdown.
func TestMain(m *testing.M) {
	var err error
	commoncli.PrintVersion(api.ProgramName)
	mainOpts, err := cli.ParseProgramFlags(os.Args[1:])

	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		os.Exit(commoncli.ExitErrParseOpts)
	}
	log := klog.NewKlogr()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	state.apiServer, err = server.NewInMemory(log, mainOpts.MinKAPIConfig)
	if err != nil {
		log.Error(err, "failed to initialize InMemoryKAPI")
		return
	}
	// Start the service in a goroutine
	go func() {
		err = state.apiServer.Start(logr.NewContext(ctx, log))
		if err != nil {
			if errors.Is(err, api.ErrStartFailed) {
				log.Error(err, "failed to start service")
			} else {
				log.Error(err, fmt.Sprintf("%s start failed", api.ProgramName))
			}
		}
	}()
	<-time.After(1 * time.Second) // give some time for startup

	err = initSuiteState()
	if err != nil {
		log.Error(err, "failed to initialize suite state")
	}

	// Run integration tests
	exitCode := m.Run()

	// Create a context with a 5-second timeout for shutdown
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Teardown: stop the server
	err = state.apiServer.Stop(shutDownCtx)
	if err != nil {
		log.Error(err, fmt.Sprintf(" %s shutdown failed", api.ProgramName))
		os.Exit(commoncli.ExitErrShutdown)
	}
	log.Info(fmt.Sprintf("%s integration shutdown gracefully.", api.ProgramName), "exitCode", exitCode)
}

func TestBaseViewCreateGetNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodesFacade := state.client.CoreV1().Nodes()

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

	watcher, err := state.client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{Watch: true})
	if err != nil {
		t.Fatalf("failed to create pods watcher: %v", err)
		return
	}
	defer watcher.Stop()

	h.wg.Add(1)
	go func() {
		listObjects(t, watcher, &h)
	}()
	createdPod, err := state.client.CoreV1().Pods("").Create(ctx, &state.podA, metav1.CreateOptions{})
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

func initSuiteState() error {
	var err error
	kubeConfigPath := state.apiServer.GetBaseView().GetKubeConfigPath()
	state.client, state.dynClient, err = clientutil.BuildClients(kubeConfigPath)
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return err
	}
	err = objutil.LoadYAMLIntoRuntimeObject("testdata/node-a.yaml", scheme, &state.nodeA)
	if err != nil {
		return err
	}
	err = objutil.LoadYAMLIntoRuntimeObject("testdata/pod-a.yaml", scheme, &state.podA)
	if err != nil {
		return err
	}
	return nil
}
