// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"fmt"
	"testing"
	"time"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/testutil"
	gocmp "github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var (
	baseViewArgs = mkapi.ViewArgs{
		Name:           "base",
		KubeConfigPath: "base",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: mkapi.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	sandboxViewArgs = mkapi.ViewArgs{
		Name:           "sandbox",
		KubeConfigPath: "sandbox",
		Scheme:         typeinfo.SupportedScheme,
		WatchConfig: mkapi.WatchConfig{
			QueueSize: mkapi.DefaultWatchQueueSize,
			Timeout:   mkapi.DefaultWatchTimeout,
		},
	}
	testNodes []corev1.Node
	testPods  []corev1.Pod
)

func TestStoreGetNode(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}

	baseChangeCount := b.GetObjectChangeCount()
	sandboxChangeCount := s.GetObjectChangeCount()
	nA := testNodes[0]
	err = storeNode(t, b, &nA, mkapi.ObjectOptions{})
	if err != nil {
		return
	}

	t.Run("CheckNodeFromBase", func(t *testing.T) {
		checkNodeInViewIsSame(t, b, &nA)
		if baseChangeCount == b.GetObjectChangeCount() {
			t.Errorf("expected base view to have changed, want %d, got %d", baseChangeCount, b.GetObjectChangeCount())
		}
	})

	t.Run("CheckBaseNodeFromSandbox", func(t *testing.T) {
		checkNodeInViewIsSame(t, s, &nA)
		if sandboxChangeCount != s.GetObjectChangeCount() {
			t.Errorf("expected sandbox view to not have changed, want %d, got %d", sandboxChangeCount, s.GetObjectChangeCount())
		}
	})

	baseChangeCount = b.GetObjectChangeCount()
	nB := *nA.DeepCopy()
	nB.GenerateName = "b-"
	err = storeNode(t, s, &nB, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		return
	}
	t.Run("CheckSandboxNodeFromSandbox", func(t *testing.T) {
		checkNodeInViewIsSame(t, s, &nB)
		if baseChangeCount != b.GetObjectChangeCount() {
			t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCount, b.GetObjectChangeCount())
		}
	})
}

func TestStoreNodeInBaseUpdateInSandbox(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, b, &nA, mkapi.ObjectOptions{})
	if err != nil {
		return
	}
	baseChangeNumAfterStore := b.GetObjectChangeCount() // mark change count of base after storing nA
	sandboxChangeNumAfterStore := s.GetObjectChangeCount()

	// lets make a copy of node A, change conditions and store in sandbox view
	nAWithCondChange := *nA.DeepCopy()
	conditions := []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionFalse,
			Reason:             "NodeNotReady",
			Message:            "Node is not ready due to some issue",
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
		},
	}
	nAWithCondChange.Status.Conditions = conditions
	err = s.UpdateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nAWithCondChange, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to update node: %v", s.GetName(), err)
		return
	}
	if baseChangeNumAfterStore != b.GetObjectChangeCount() {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeNumAfterStore, b.GetObjectChangeCount())
	}

	// get node from base
	nABase, err := getNode(t, b, nAWithCondChange.GetName())
	if err != nil {
		return
	}
	checkNodeInViewIsSame(t, b, nABase) // check that node in base view has not changed after updating in sandbox
	if t.Failed() {
		return
	}

	// check node in sandbox view is ihe same as the node after changing conditions
	checkNodeInViewIsSame(t, s, &nAWithCondChange)
	if sandboxChangeNumAfterStore == s.GetObjectChangeCount() {
		t.Errorf("expected sandbox view to have diff change count, got same %d", s.GetObjectChangeCount())
		return
	}
}

func TestSandboxPodSandboxNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	initialBaseChangeCount := b.GetObjectChangeCount()
	nA := testNodes[0]
	err = storeNode(t, s, &nA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		return
	}
	pA := testPods[0]
	err = storePod(t, s, &pA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		return
	}
	pAModified, err := updateBinding(t, s, &pA, &nA) // update pod-node binding in the sandbox view
	if err != nil {
		return
	}
	pAModName := objutil.CacheName(pAModified)

	if pAModified.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", b.GetName(), nA.GetName(), pAModified.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pASandbox.Name != pA.GetName() && pASandbox.Namespace != pAModified.GetNamespace() {
		t.Errorf("in %q view, expected pod %q,  got %q", b.GetName(), pASandboxName, pAModName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if pABase != nil {
		t.Errorf("in %q view, expected pod %q to not exist, got %q", b.GetName(), pAModName, pABase.GetName())
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("in %q view, expected pod %q to not exist, got %v", b.GetName(), pAModName, err)
	}
	if b.GetObjectChangeCount() != initialBaseChangeCount {
		t.Errorf("expected base view to not have changed, want %d, got %d", initialBaseChangeCount, b.GetObjectChangeCount())
	}
}

func TestSandboxPodBaseNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, b, &nA, mkapi.ObjectOptions{}) //store node in base
	if err != nil {
		return
	}
	baseChangeCountAfterNodeStore := b.GetObjectChangeCount()
	pA := testPods[0]
	err = storePod(t, s, &pA, mkapi.ObjectOptions{NoBroadcast: true}) // store pod in sandbox
	if err != nil {
		return
	}
	pAModified, err := updateBinding(t, s, &pA, &nA) // update pod-node binding via sandbox view
	if err != nil {
		return
	}
	pAModName := objutil.CacheName(pAModified)

	if pAModified.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", b.GetName(), nA.GetName(), pAModified.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pASandbox.Name != pA.GetName() && pASandbox.Namespace != pAModified.GetNamespace() {
		t.Errorf("In %q view, expected pod %q,  got %q", b.GetName(), pASandboxName, pAModName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if err == nil {
		t.Fatalf("in %q view, expected pod %q to not exist, got no error", b.GetName(), pAModName)
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("in %q view, expected pod %q to not exist, got %v", b.GetName(), pAModName, err)
		return
	}
	if pABase != nil {
		t.Fatalf("in %q view, expected pod %q to not exist, got %q", b.GetName(), pAModName, pABase)
		return
	}
	if b.GetObjectChangeCount() != baseChangeCountAfterNodeStore {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCountAfterNodeStore, b.GetObjectChangeCount())
	}
}

func TestBasePodSandboxNodeBinding(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nA := testNodes[0]
	err = storeNode(t, s, &nA, mkapi.ObjectOptions{NoBroadcast: true}) //store node in the sandbox view
	if err != nil {
		return
	}
	pA := testPods[0]
	err = storePod(t, b, &pA, mkapi.ObjectOptions{}) // store pod in the base view
	if err != nil {
		return
	}
	baseChangeCountAfterPodStore := b.GetObjectChangeCount()

	pAUpdated, err := updateBinding(t, s, &pA, &nA) // update pod-node binding in the  sandbox view
	if err != nil {
		return
	}
	pAUpdatedName := objutil.CacheName(pAUpdated)

	if pAUpdated.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pAUpdated.Spec.NodeName)
	}
	pASandbox, err := getPod(t, s, pA.GetNamespace(), pA.GetName())
	if err != nil {
		return
	}
	pASandboxName := objutil.CacheName(pASandbox)
	if pASandbox.Spec.NodeName != nA.GetName() {
		t.Errorf("in %q view, expected pod to be bound to node %q, got %q", s.GetName(), nA.GetName(), pASandbox.Spec.NodeName)
	}
	if pAUpdatedName != pASandboxName {
		t.Errorf("In %q view, expected pod %q,  got %q", b.GetName(), pAUpdatedName, pASandboxName)
	}

	pABase, err := getPod(t, b, pA.GetNamespace(), pA.GetName())
	if err != nil {
		t.Errorf("in %q view, expected pod %q to exist, got %v", b.GetName(), pAUpdatedName, err)
		return
	}
	if pABase.Spec.NodeName != "" {
		t.Errorf("in %q view, expected pod %q to not be bound to a node, got %q", b.GetName(), pAUpdatedName, pABase.Spec.NodeName)
	}
	if b.GetObjectChangeCount() != baseChangeCountAfterPodStore {
		t.Errorf("expected base view to not have changed, want %d, got %d", baseChangeCountAfterPodStore, b.GetObjectChangeCount())
	}
}

func setup(t *testing.T) (b mkapi.View, s mkapi.View, err error) {
	t.Helper()
	err = loadTestNodes(t)
	if err != nil {
		return
	}
	err = loadTestPods(t)
	if err != nil {
		return
	}
	b, s, err = createViews(t)
	t.Cleanup(func() {
		err = s.Close()
		if err != nil {
			t.Logf("failed to close sandbox view: %v", err)
		}
		err = b.Close()
		if err != nil {
			t.Logf("failed to close base view: %v", err)
		}
	})
	return
}

func loadTestNodes(t *testing.T) error {
	t.Helper()
	var err error
	if testNodes != nil {
		return nil
	}
	testNodes, err = testutil.LoadTestNodes()
	if err != nil {
		t.Fatalf("failed to load test nodes: %v", err)
		return err
	}
	return nil
}

func loadTestPods(t *testing.T) error {
	t.Helper()
	var err error
	if testPods != nil {
		return nil
	}
	testPods, err = testutil.LoadTestPods()
	if err != nil {
		t.Fatalf("failed to load test pods: %v", err)
		return err
	}
	return nil
}

func createViews(t *testing.T) (b mkapi.View, s mkapi.View, err error) {
	t.Helper()
	b, err = NewBase(&baseViewArgs)
	if err != nil {
		t.Fatalf("failed to create base view: %v", err)
		return
	}
	s, err = NewSandbox(b, &sandboxViewArgs)
	if err != nil {
		t.Fatalf("failed to create sandbox view: %v", err)
		return
	}
	return
}

func updateBinding(t *testing.T, v mkapi.View, p *corev1.Pod, n *corev1.Node) (*corev1.Pod, error) {
	t.Helper()
	binding := createBinding(p, n)
	pMod, err := v.UpdatePodNodeBinding(t.Context(), objutil.CacheName(p), binding)
	if err != nil {
		t.Fatalf("failed to update pod node binding: %v", err)
		return nil, err
	}
	return pMod, nil
}

func createBinding(p *corev1.Pod, n *corev1.Node) corev1.Binding {
	return corev1.Binding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Binding",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.GetName(),
			Namespace: p.GetNamespace(),
			UID:       p.GetUID(),
		},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: n.GetName(),
		},
	}
}

func storePod(t *testing.T, v mkapi.View, p *corev1.Pod, opts mkapi.ObjectOptions) error {
	t.Helper()
	_, err := v.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, p, opts)
	if err != nil {
		t.Fatalf("failed to store pod: %v", err)
		return err
	}
	return nil
}

func storeNode(t *testing.T, v mkapi.View, n *corev1.Node, opts mkapi.ObjectOptions) error {
	t.Helper()
	_, err := v.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, n, opts)
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", v.GetName(), err)
		return err
	}
	return nil
}

func getNode(t *testing.T, v mkapi.View, name string) (n *corev1.Node, err error) {
	t.Helper()
	o, err := v.GetObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", name))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		t.Fatalf("from view %q, failed to get node: %v", v.GetName(), err)
		return
	}
	n, ok := o.(*corev1.Node)
	if !ok {
		err = fmt.Errorf("expected Node, got %T", o)
		t.Fatalf("from view %q, failed to get node: %v", v.GetName(), err)
	}
	return
}

func getPod(t *testing.T, v mkapi.View, namespace, name string) (p *corev1.Pod, err error) {
	t.Helper()
	o, err := v.GetObject(t.Context(), typeinfo.PodsDescriptor.GVK, cache.NewObjectName(namespace, name))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		t.Fatalf("failed to get Pod: %v", err)
		return
	}
	p, ok := o.(*corev1.Pod)
	if !ok {
		err = fmt.Errorf("expected Pod, got %T", o)
		t.Fatalf("failed to get Pod: %v", err)
	}
	return
}

func checkNodeInViewIsSame(t *testing.T, v mkapi.View, n *corev1.Node) {
	t.Helper()
	nAFromView, err := getNode(t, v, n.GetName())
	if err != nil {
		t.Fatalf("From view %q, failed to get node: %v", v.GetName(), err)
		return
	}
	if n.GetName() != nAFromView.GetName() {
		t.Errorf("From view %q, expected node %q, got %q", v.GetName(), n.GetName(), nAFromView.GetName())
	}
	diff := gocmp.Diff(n, nAFromView)
	if diff != "" {
		t.Errorf("From view %q, expected node spec for %q to be same, got diff: %s", v.GetName(), n.GetName(), diff)
	}
}

// ---- Watch duplicate-event tests --------------------------------------------

// TestSandboxWatchObjects_NoDuplicateOnCopyOnWriteUpdate verifies that the callback-style
// WatchObjects API delivers exactly one event per logical update, even when the update is on a
// delegate-only object (which triggers the sandbox's copy-on-write path). The expected sequence:
//
//   - One ADD on watcher attach (initial-list replay).
//   - One MODIFIED for each subsequent UpdateObject call.
//
// In particular, no second ADD or MODIFIED is delivered as a side-effect of the materialize step
// because materialize uses NoBroadcast=true, and the delegate watcher's events are filtered by
// WatchObjects' shadowing wrapper once the sandbox holds a local copy.
func TestSandboxWatchObjects_NoDuplicateOnCopyOnWriteUpdate(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}

	// Pre-populate a node in the BASE view (so an Update on the sandbox triggers copy-on-write).
	nA := testNodes[0]
	if err = storeNode(t, b, &nA, mkapi.ObjectOptions{}); err != nil {
		return
	}

	events := startWatchObjects(t, s)

	// Drain the initial ADD that arrives on watcher attach.
	expectEvent(t, events, watch.Added, nA.Name)

	// Update the node via the sandbox: copy-on-write into sandbox, then update.
	nAv2 := *nA.DeepCopy()
	if nAv2.Annotations == nil {
		nAv2.Annotations = map[string]string{}
	}
	nAv2.Annotations["v"] = "2"
	if err = s.UpdateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nAv2, mkapi.ObjectOptions{}); err != nil {
		t.Fatalf("UpdateObject failed: %v", err)
	}
	expectEvent(t, events, watch.Modified, nA.Name)
	expectNoMoreEvents(t, events)

	// Second update: object is now sandbox-local, no copy-on-write.
	nAv3 := *nAv2.DeepCopy()
	nAv3.Annotations["v"] = "3"
	if err = s.UpdateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nAv3, mkapi.ObjectOptions{}); err != nil {
		t.Fatalf("second UpdateObject failed: %v", err)
	}
	expectEvent(t, events, watch.Modified, nA.Name)
	expectNoMoreEvents(t, events)
}

// TestSandboxGetWatcher_NoDuplicateOnCopyOnWriteUpdate verifies the same property for the
// channel-style GetWatcher API. Unlike WatchObjects, GetWatcher does not filter delegate events
// — it merges sandbox+delegate watch streams via watchutil.CombineTwoWatchers. So this test is
// the more pessimistic of the two: if duplicate events were a real problem in the kube-scheduler
// path (which uses GetWatcher via inmclient), this is where they would surface.
//
// We expect no duplicates because in the copy-on-write Update flow, the materialize step uses
// NoBroadcast=true (so the sandbox watcher does not fire) and the delegate watcher only fires
// when the delegate's data actually changes — it does not, since UpdateObject only touches the
// sandbox copy. So the consumer sees one ADD on attach and one MODIFIED per Update.
func TestSandboxGetWatcher_NoDuplicateOnCopyOnWriteUpdate(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}

	nA := testNodes[0]
	if err = storeNode(t, b, &nA, mkapi.ObjectOptions{}); err != nil {
		return
	}

	w, err := s.GetWatcher(t.Context(), typeinfo.NodesDescriptor.GVK, "", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("GetWatcher failed: %v", err)
	}
	t.Cleanup(w.Stop)
	events := w.ResultChan()

	expectEventFromChan(t, events, watch.Added, nA.Name)

	nAv2 := *nA.DeepCopy()
	if nAv2.Annotations == nil {
		nAv2.Annotations = map[string]string{}
	}
	nAv2.Annotations["v"] = "2"
	if err = s.UpdateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nAv2, mkapi.ObjectOptions{}); err != nil {
		t.Fatalf("UpdateObject failed: %v", err)
	}
	expectEventFromChan(t, events, watch.Modified, nA.Name)
	expectNoMoreEventsFromChan(t, events)

	nAv3 := *nAv2.DeepCopy()
	nAv3.Annotations["v"] = "3"
	if err = s.UpdateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nAv3, mkapi.ObjectOptions{}); err != nil {
		t.Fatalf("second UpdateObject failed: %v", err)
	}
	expectEventFromChan(t, events, watch.Modified, nA.Name)
	expectNoMoreEventsFromChan(t, events)
}

// startWatchObjects spawns the sandbox view's blocking WatchObjects in a goroutine and returns
// a channel onto which received events are forwarded. The watch is bound to the test context so
// it terminates with the test.
func startWatchObjects(t *testing.T, v mkapi.View) chan watch.Event {
	t.Helper()
	ch := make(chan watch.Event, 16)
	go func() {
		_ = v.WatchObjects(t.Context(), typeinfo.NodesDescriptor.GVK, 0, "", labels.Everything(), func(ev watch.Event) error {
			ch <- ev
			return nil
		})
		close(ch)
	}()
	return ch
}

// expectEvent reads one event from ch within a short timeout and verifies its type and the
// affected object's name. Fails the test if nothing arrives in time or the event is wrong.
func expectEvent(t *testing.T, ch <-chan watch.Event, wantType watch.EventType, wantName string) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("watch channel closed; expected %s event for %q", wantType, wantName)
		}
		if ev.Type != wantType {
			t.Fatalf("expected event type %s, got %s", wantType, ev.Type)
		}
		gotName, _ := objutil.AsMeta(ev.Object)
		if gotName != nil && gotName.GetName() != wantName {
			t.Fatalf("expected event for %q, got %q", wantName, gotName.GetName())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s event for %q", wantType, wantName)
	}
}

// expectNoMoreEvents asserts that no further event arrives within a short window. The window is
// short enough to keep the test fast but long enough to surface a duplicate that's already been
// queued by an alternate watcher.
func expectNoMoreEvents(t *testing.T, ch <-chan watch.Event) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			return
		}
		gotName, _ := objutil.AsMeta(ev.Object)
		name := ""
		if gotName != nil {
			name = gotName.GetName()
		}
		t.Fatalf("unexpected duplicate event: type=%s name=%q", ev.Type, name)
	case <-time.After(150 * time.Millisecond):
		// no event — pass
	}
}

// expectEventFromChan / expectNoMoreEventsFromChan are the GetWatcher-channel equivalents of
// the WatchObjects-channel helpers. They are split out only because watch.Interface returns a
// receive-only channel via ResultChan.
func expectEventFromChan(t *testing.T, ch <-chan watch.Event, wantType watch.EventType, wantName string) {
	t.Helper()
	expectEvent(t, ch, wantType, wantName)
}

func expectNoMoreEventsFromChan(t *testing.T, ch <-chan watch.Event) {
	t.Helper()
	expectNoMoreEvents(t, ch)
}
