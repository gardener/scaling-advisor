// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"fmt"
	"testing"
	"time"

	viewtestutil "github.com/gardener/scaling-advisor/minkapi/view/testutil"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	"github.com/gardener/scaling-advisor/common/objutil"
	gocmp "github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func TestStoreGetNode(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}

	baseChangeCount := b.GetObjectChangeCount()
	sandboxChangeCount := s.GetObjectChangeCount()
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}

	t.Run("CheckNodeFromBase", func(t *testing.T) {
		checkNodeInViewIsSame(t, b, nA)
		if baseChangeCount == b.GetObjectChangeCount() {
			t.Errorf("expected base view to have changed, want %d, got %d", baseChangeCount, b.GetObjectChangeCount())
		}
	})

	t.Run("CheckBaseNodeFromSandbox", func(t *testing.T) {
		checkNodeInViewIsSame(t, s, nA)
		if sandboxChangeCount != s.GetObjectChangeCount() {
			t.Errorf("expected sandbox view to not have changed, want %d, got %d", sandboxChangeCount, s.GetObjectChangeCount())
		}
	})

	baseChangeCount = b.GetObjectChangeCount()
	nB := *nA.DeepCopy()
	nB.GenerateName = "b-"
	_, err = s.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nB, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", s.GetName(), err)
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
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
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
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = s.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", s.GetName(), err)
		return
	}
	pAObj, ok := viewtestutil.GetObject(viewtestutil.PodA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.PodA)
	}
	pA := pAObj.(*corev1.Pod)
	_, err = s.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, pA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("in view %q, failed to store pod: %v", s.GetName(), err)
		return
	}
	pAModified, err := updateBinding(t, s, pA, nA) // update pod-node binding in the sandbox view
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
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	baseChangeCountAfterNodeStore := b.GetObjectChangeCount()
	pAObj, ok := viewtestutil.GetObject(viewtestutil.PodA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.PodA)
	}
	pA := pAObj.(*corev1.Pod)
	_, err = s.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, pA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("in view %q, failed to store pod: %v", s.GetName(), err)
		return
	}
	pAModified, err := updateBinding(t, s, pA, nA) // update pod-node binding via sandbox view
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
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = s.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", s.GetName(), err)
		return
	}
	pAObj, ok := viewtestutil.GetObject(viewtestutil.PodA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.PodA)
	}
	pA := pAObj.(*corev1.Pod)
	_, err = b.CreateObject(t.Context(), typeinfo.PodsDescriptor.GVK, pA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store pod: %v", b.GetName(), err)
		return
	}
	baseChangeCountAfterPodStore := b.GetObjectChangeCount()

	pAUpdated, err := updateBinding(t, s, pA, nA) // update pod-node binding in the  sandbox view
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
	b, err = NewBase(viewtestutil.GetDefaultBaseViewArgs())
	if err != nil {
		t.Fatalf("failed to create base view: %v", err)
		return
	}
	s, err = NewSandbox(b, viewtestutil.GetDefaultSandboxViewArgs())
	if err != nil {
		t.Fatalf("failed to create sandbox view: %v", err)
		return
	}
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

func updateBinding(t *testing.T, v mkapi.View, p *corev1.Pod, n *corev1.Node) (*corev1.Pod, error) {
	t.Helper()
	binding := corev1.Binding{
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
	pMod, err := v.UpdatePodNodeBinding(t.Context(), objutil.CacheName(p), binding)
	if err != nil {
		t.Fatalf("failed to update pod node binding: %v", err)
		return nil, err
	}
	return pMod, nil
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

// ---- Tombstone tests --------------------------------------------------------

// TestTombstone_GetReturnsNotFound verifies that after deleting a delegate-only object via the
// sandbox, GetObject on the sandbox returns the standard apierrors NotFound — the tombstone
// hides the delegate's copy from the sandbox's perspective.
func TestTombstone_GetReturnsNotFound(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	if _, err := getNode(t, s, nA.Name); !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound from sandbox after delete, got: %v", err)
	}
}

// TestTombstone_DelegateUntouched verifies that DeleteObject on a sandbox view does NOT mutate
// the delegate: the same object remains visible from the base view even after the sandbox has
// tombstoned it.
func TestTombstone_DelegateUntouched(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	baseChangeBefore := b.GetObjectChangeCount()

	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	if got, err := getNode(t, b, nA.Name); err != nil || got == nil {
		t.Errorf("expected base view to still hold node %q after sandbox delete, got err=%v", nA.Name, err)
	}
	if b.GetObjectChangeCount() != baseChangeBefore {
		t.Errorf("expected base view change count to be unchanged (%d) after sandbox delete, got %d",
			baseChangeBefore, b.GetObjectChangeCount())
	}
}

// TestTombstone_ListExcludesTombstoned verifies that ListNodes on the sandbox view returns
// every base-view node EXCEPT the ones tombstoned by sandbox-side deletes.
func TestTombstone_ListExcludesTombstoned(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	nB := *nA.DeepCopy()
	nB.Name = nA.Name + "-second"
	nB.ResourceVersion = ""
	nB.UID = ""
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nB, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}

	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	nodes, err := s.ListNodes(t.Context())
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	for _, n := range nodes {
		if n.Name == nA.Name {
			t.Errorf("expected sandbox ListNodes to exclude tombstoned node %q, got %v", nA.Name, nodeNames(nodes))
		}
	}
	hasB := false
	for _, n := range nodes {
		if n.Name == nB.Name {
			hasB = true
			break
		}
	}
	if !hasB {
		t.Errorf("expected sandbox ListNodes to still include non-tombstoned node %q, got %v", nB.Name, nodeNames(nodes))
	}
}

// TestTombstone_RecreateClearsTombstone verifies that a CreateObject for a previously
// tombstoned name resurrects the object: subsequent reads see the new state, not NotFound.
func TestTombstone_RecreateClearsTombstone(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	// Re-Create with a marker annotation so we can confirm the new object is what we read back.
	nFresh := *nA.DeepCopy()
	nFresh.ResourceVersion = ""
	nFresh.UID = ""
	if nFresh.Annotations == nil {
		nFresh.Annotations = map[string]string{}
	}
	nFresh.Annotations["recreated"] = "yes"
	_, err = s.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, &nFresh, mkapi.ObjectOptions{NoBroadcast: true})
	if err != nil {
		t.Fatalf("re-CreateObject failed: %v", err)
		return
	}

	got, err := getNode(t, s, nA.Name)
	if err != nil {
		t.Fatalf("expected sandbox to return the re-created node, got err=%v", err)
	}
	if got.Annotations["recreated"] != "yes" {
		t.Errorf("expected re-created annotation marker, got annotations=%v", got.Annotations)
	}
}

// TestTombstone_DoubleDeleteReturnsNotFound verifies that DELETE-ing an already-tombstoned name
// returns NotFound (mirroring K8s API server behavior for delete-on-missing).
func TestTombstone_DoubleDeleteReturnsNotFound(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}
	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("first DeleteObject failed: %v", err)
	}
	err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name))
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound on second DeleteObject of tombstoned name, got: %v", err)
	}
}

// TestTombstone_WatchObjectsDeliversDeletedThenSilence verifies that the WatchObjects callback
// API delivers exactly one Deleted event when a delegate-only object is tombstoned, and no
// further events arrive within a short window (the delegate watcher's events are filtered out).
func TestTombstone_WatchObjectsDeliversDeletedThenSilence(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}

	events := startWatchObjects(t, s)
	// Drain initial-list ADDs; the kubernetes Broadcaster can deliver an already-queued
	// Action() to a newly-attached watcher in addition to the watcher's own initial-list
	// prefix, so the count is non-deterministic but always at least 1.
	if got := drainAdded(t, events); got < 1 {
		t.Fatalf("expected at least 1 initial-list ADDED event for %q, got %d", nA.Name, got)
	}

	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	expectEvent(t, events, watch.Deleted, nA.Name)
	expectNoMoreEvents(t, events)
}

// TestTombstone_GetWatcherDeliversDeletedThenSilence is the channel-style equivalent of the
// previous test — checks the GetWatcher path (used by inmclient) for the same property.
func TestTombstone_GetWatcherDeliversDeletedThenSilence(t *testing.T) {
	b, s, err := setup(t)
	if err != nil {
		return
	}
	nAObj, ok := viewtestutil.GetObject(viewtestutil.NodeA)
	if !ok {
		t.Fatalf("test object %q not found", viewtestutil.NodeA)
	}
	nA := nAObj.(*corev1.Node)
	_, err = b.CreateObject(t.Context(), typeinfo.NodesDescriptor.GVK, nA, mkapi.ObjectOptions{})
	if err != nil {
		t.Fatalf("in view %q, failed to store node: %v", b.GetName(), err)
		return
	}

	w, err := s.GetWatcher(t.Context(), typeinfo.NodesDescriptor.GVK, "", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("GetWatcher failed: %v", err)
	}
	t.Cleanup(w.Stop)
	events := w.ResultChan()

	if got := drainAdded(t, events); got < 1 {
		t.Fatalf("expected at least 1 initial-list ADDED event for %q, got %d", nA.Name, got)
	}

	if err = s.DeleteObject(t.Context(), typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nA.Name)); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	expectEvent(t, events, watch.Deleted, nA.Name)
	expectNoMoreEvents(t, events)
}

// drainAdded reads ADDED events from ch until none arrive within a short window. Used after
// attaching a watcher to swallow the (possibly duplicated) initial-list replays — the
// kubernetes Broadcaster can deliver an Action()'d event to a newly-attached watcher if the
// Action hasn't been distributed yet, on top of the watcher's own initial-list prefix.
func drainAdded(t *testing.T, ch <-chan watch.Event) int {
	t.Helper()
	count := 0
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return count
			}
			if ev.Type != watch.Added {
				t.Fatalf("drainAdded: unexpected non-Added event %s before drain completed", ev.Type)
			}
			count++
		case <-time.After(150 * time.Millisecond):
			return count
		}
	}
}

func nodeNames(nodes []corev1.Node) []string {
	names := make([]string, 0, len(nodes))
	for i := range nodes {
		names = append(names, nodes[i].Name)
	}
	return names
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
		gotMeta, _ := objutil.AsMeta(ev.Object)
		if gotMeta != nil && gotMeta.GetName() != wantName {
			t.Fatalf("expected event for %q, got %q", wantName, gotMeta.GetName())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s event for %q", wantType, wantName)
	}
}

// expectNoMoreEvents asserts that no further event arrives within a short window.
func expectNoMoreEvents(t *testing.T, ch <-chan watch.Event) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			return
		}
		gotMeta, _ := objutil.AsMeta(ev.Object)
		name := ""
		if gotMeta != nil {
			name = gotMeta.GetName()
		}
		t.Fatalf("unexpected duplicate event: type=%s name=%q", ev.Type, name)
	case <-time.After(150 * time.Millisecond):
		// no event — pass
	}
}
